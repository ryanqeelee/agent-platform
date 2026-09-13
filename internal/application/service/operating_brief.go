package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const (
	operatingBriefQueryLimit   = 10000
	operatingBriefMaxWeekRows  = 50000
	operatingBriefRetrySeconds = 2
)

var operatingBriefReadinessColumns = []string{
	"retail_week_start", "retail_week_end_exclusive", "business_timezone", "readiness_status",
	"financial_close_status", "closed_at", "financial_close_evidence_ref", "ready_at", "roster_version",
	"roster_sha256", "metric_versions_ref", "metric_versions_sha256", "evidence_mode", "evidence_ref",
	"evidence_sha256", "snapshot_sha256", "partition_sha256", "partition_row_count", "source_snapshot_ref",
	"publication_sha256", "revision", "fact_grain", "store_id", "store_name", "category_id", "category_name",
	"coverage_status", "sales_fact_row_count", "net_sales_amount", "cost_amount", "margin_amount",
	"expected_store_count", "observed_store_count", "missing_store_count",
}

type OperatingBriefService struct {
	repo     apprepo.OperatingBriefRepository
	members  interfaces.TenantMemberService
	tenants  interfaces.TenantService
	resolver interfaces.GovernedEdgeResolver
	queue    interfaces.TaskEnqueuer
	compute  interfaces.OperatingBriefComputeClient
	now      func() time.Time
}

func NewOperatingBriefService(
	repo apprepo.OperatingBriefRepository,
	members interfaces.TenantMemberService,
	tenants interfaces.TenantService,
	resolver interfaces.GovernedEdgeResolver,
	queue interfaces.TaskEnqueuer,
	compute interfaces.OperatingBriefComputeClient,
) *OperatingBriefService {
	return &OperatingBriefService{
		repo: repo, members: members, tenants: tenants,
		resolver: resolver, queue: queue, compute: compute, now: time.Now,
	}
}

func (s *OperatingBriefService) TaskType() string { return types.TypeOperatingBriefRefresh }

func (s *OperatingBriefService) Read(ctx context.Context, scopeRef string) (map[string]any, error) {
	permission, err := CurrentOperatingAnalysisReadPermission(ctx, s.members, s.tenants)
	if err != nil {
		return nil, err
	}
	if !permission.Allowed() {
		return nil, agenttools.ErrGovernedDataAccessDenied
	}
	tenantID, _ := types.TenantIDFromContext(ctx)
	actorID, _ := types.UserIDFromContext(ctx)
	scope, label, err := s.resolveScope(ctx, tenantID, scopeRef)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.repo.LatestSnapshot(ctx, tenantID, scope)
	if err != nil {
		return nil, err
	}
	refresh, err := s.repo.LatestRefresh(ctx, tenantID, scope)
	if err != nil {
		return nil, err
	}
	if snapshot == nil && !briefRefreshActive(refresh) {
		if _, err := s.enqueue(ctx, tenantID, actorID, scope); err != nil {
			return nil, err
		}
		refresh, err = s.repo.LatestRefresh(ctx, tenantID, scope)
		if err != nil {
			return nil, err
		}
	}
	if snapshot != nil {
		return s.publicSnapshot(ctx, snapshot, scopeRef, label, refresh)
	}
	return s.emptyPublic(scopeRef, label, refresh), nil
}

func (s *OperatingBriefService) EnqueueRefresh(ctx context.Context, scopeRef string) error {
	permission, err := CurrentOperatingAnalysisReadPermission(ctx, s.members, s.tenants)
	if err != nil || !permission.Allowed() {
		if err != nil {
			return err
		}
		return agenttools.ErrGovernedDataAccessDenied
	}
	tenantID, _ := types.TenantIDFromContext(ctx)
	actorID, _ := types.UserIDFromContext(ctx)
	scope, _, err := s.resolveScope(ctx, tenantID, scopeRef)
	if err != nil {
		return err
	}
	_, err = s.enqueue(ctx, tenantID, actorID, scope)
	return err
}

func briefRefreshActive(refresh *types.OperatingBriefRefresh) bool {
	return refresh != nil && (refresh.Status == types.OperatingBriefRefreshQueued || refresh.Status == types.OperatingBriefRefreshRunning)
}

func (s *OperatingBriefService) CreateHandoff(ctx context.Context, snapshotRef, anchorRef string) (map[string]any, error) {
	permission, err := CurrentOperatingAnalysisReadPermission(ctx, s.members, s.tenants)
	if err != nil || !permission.Allowed() {
		if err != nil {
			return nil, err
		}
		return nil, agenttools.ErrGovernedDataAccessDenied
	}
	tenantID, _ := types.TenantIDFromContext(ctx)
	question, err := s.repo.SnapshotQuestion(ctx, tenantID, snapshotRef, anchorRef)
	if err != nil {
		return nil, err
	}
	return map[string]any{"schema": "OperatingAnalysisHandoffV1", "question": question}, nil
}

func (s *OperatingBriefService) enqueue(ctx context.Context, tenantID uint64, actorID string, scope types.OperatingBriefScope) (bool, error) {
	now := s.now().UTC()
	refresh := &types.OperatingBriefRefresh{
		ID: uuid.NewString(), TenantID: tenantID, RequesterUserID: actorID,
		ScopeKind: scope.Kind, StoreID: scope.StoreID,
		Status: types.OperatingBriefRefreshQueued, CreatedAt: now, UpdatedAt: now,
	}
	created, err := s.repo.CreateRefresh(ctx, refresh)
	if err != nil || !created {
		return created, err
	}
	payload, err := json.Marshal(types.OperatingBriefRefreshPayload{
		TenantID: tenantID, RequesterUserID: actorID, Scope: scope, RefreshID: refresh.ID,
	})
	if err != nil {
		return false, err
	}
	_, err = s.queue.Enqueue(asynq.NewTask(types.TypeOperatingBriefRefresh, payload),
		asynq.Queue(types.QueueMaintenance), asynq.MaxRetry(0), asynq.Timeout(10*time.Minute))
	if err != nil {
		_ = s.repo.MarkRefreshFailed(ctx, refresh.ID, "queue_unavailable")
		return false, err
	}
	return true, nil
}

func (s *OperatingBriefService) Handle(ctx context.Context, task *asynq.Task) error {
	var payload types.OperatingBriefRefreshPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil || !validBriefPayload(payload) {
		return fmt.Errorf("invalid operating brief task")
	}
	if err := s.repo.MarkRefreshRunning(ctx, payload.RefreshID); err != nil {
		return err
	}
	if err := s.refresh(ctx, payload); err != nil {
		_ = s.repo.MarkRefreshFailed(ctx, payload.RefreshID, "refresh_failed")
		return err
	}
	return nil
}

func validBriefPayload(p types.OperatingBriefRefreshPayload) bool {
	return p.TenantID != 0 && strings.TrimSpace(p.RequesterUserID) != "" && strings.TrimSpace(p.RefreshID) != "" &&
		((p.Scope.Kind == types.OperatingBriefScopeAll && p.Scope.StoreID == "") ||
			(p.Scope.Kind == types.OperatingBriefScopeStore && strings.TrimSpace(p.Scope.StoreID) != ""))
}

func (s *OperatingBriefService) refresh(ctx context.Context, payload types.OperatingBriefRefreshPayload) error {
	workerCtx := context.WithValue(ctx, types.TenantIDContextKey, payload.TenantID)
	workerCtx = context.WithValue(workerCtx, types.UserIDContextKey, payload.RequesterUserID)
	workerCtx = types.WithPrincipal(workerCtx, types.Principal{Type: types.PrincipalWebUser, ID: payload.RequesterUserID})
	collectionTime := s.now().UTC()
	permission, err := CurrentOperatingAnalysisReadPermission(workerCtx, s.members, s.tenants)
	if err != nil || !permission.Allowed() {
		if err != nil {
			return err
		}
		return agenttools.ErrGovernedDataAccessDenied
	}
	connection, err := s.resolver.Resolve(workerCtx, payload.TenantID)
	if err != nil {
		return err
	}
	client, err := agenttools.NewGovernedDataClient(connection, func(checkCtx context.Context) error {
		return s.authorizeBriefConnection(checkCtx, payload.TenantID, connection)
	})
	if err != nil {
		return err
	}
	catalogIndex, err := client.Catalog(workerCtx)
	if err != nil {
		return err
	}
	authority, err := discoverBriefAuthority(workerCtx, client, catalogIndex)
	if err != nil {
		return err
	}
	roles := map[string]string{
		"store_dimension":          authority.storeTable,
		"operating_week_readiness": authority.readinessTable,
	}
	storeDefinition, readinessDefinition := authority.storeDefinition, authority.readinessDefinition
	catalogBundle, err := briefCatalogBundle(catalogIndex, storeDefinition, readinessDefinition)
	if err != nil {
		return err
	}
	sources, _ := catalogBundle["sources"].([]any)
	if len(sources) != 1 {
		return errors.New("invalid operating brief catalog source")
	}
	source, _ := sources[0].(map[string]any)
	database, _ := source["database"].(string)
	if !briefSQLIdentifier(database) || !briefSQLIdentifier(roles["store_dimension"]) || !briefSQLIdentifier(roles["operating_week_readiness"]) {
		return errors.New("invalid operating brief catalog identifiers")
	}
	storeView := database + "." + roles["store_dimension"]
	readinessView := database + "." + roles["operating_week_readiness"]
	roster, err := briefQueryRows(workerCtx, client, "SELECT store_id, store_name, portfolio_version FROM "+storeView+" ORDER BY store_id", payload.RefreshID+":roster")
	if err != nil {
		return err
	}
	weeks := fixedBriefWeeks(collectionTime)
	weekRows := make(map[string]any, len(weeks))
	for _, week := range weeks {
		sql := "SELECT " + strings.Join(operatingBriefReadinessColumns, ", ") + " FROM " + readinessView +
			" WHERE retail_week_start = '" + week + "' ORDER BY fact_grain, store_id, category_id"
		rows, err := briefQueryRows(workerCtx, client, sql, payload.RefreshID+":"+week)
		if err != nil {
			return err
		}
		weekRows[week] = rows
	}
	computeResult, err := s.compute.Compute(workerCtx, map[string]any{
		"contractVersion": "operating-brief-bundle/1", "sourceId": connection.SourceID,
		"catalog": catalogBundle, "asOf": collectionTime.Format(time.RFC3339Nano),
		"scope": briefComputeScope(payload.Scope), "rosterRows": roster, "weekRows": weekRows,
	})
	if err != nil {
		return err
	}
	return s.persistProjection(workerCtx, payload, connection, catalogIndex, computeResult)
}

func (s *OperatingBriefService) authorizeBriefConnection(ctx context.Context, tenantID uint64, expected types.GovernedEdgeConnection) error {
	permission, err := CurrentOperatingAnalysisReadPermission(ctx, s.members, s.tenants)
	if err != nil {
		return err
	}
	if !permission.Allowed() {
		return agenttools.ErrGovernedDataAccessDenied
	}
	current, err := s.resolver.Resolve(ctx, tenantID)
	if err != nil {
		return err
	}
	if current != expected {
		return agenttools.ErrGovernedDataAccessDenied
	}
	return nil
}

func briefComputeScope(scope types.OperatingBriefScope) map[string]any {
	if scope.Kind == types.OperatingBriefScopeStore {
		return map[string]any{"kind": "store", "storeId": scope.StoreID}
	}
	return map[string]any{"kind": "all_authorized"}
}

func fixedBriefWeeks(now time.Time) []string {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	today := now.In(location)
	current := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, location).
		AddDate(0, 0, -((int(today.Weekday())+6)%7)-7)
	weeks := make([]string, 0, 9)
	for offset := 0; offset < 8; offset++ {
		weeks = append(weeks, current.AddDate(0, 0, -7*offset).Format("2006-01-02"))
	}
	weeks = append(weeks, current.AddDate(0, 0, -364).Format("2006-01-02"))
	return weeks
}

type briefAuthority struct {
	storeTable          string
	readinessTable      string
	storeDefinition     map[string]any
	readinessDefinition map[string]any
}

func discoverBriefAuthority(ctx context.Context, client *agenttools.GovernedDataClient, index map[string]any) (briefAuthority, error) {
	source, _ := index["source"].(map[string]any)
	tables := anySlice(source["tables"])
	if len(tables) == 0 {
		return briefAuthority{}, errors.New("operating brief catalog has no tables")
	}
	var authority briefAuthority
	for _, raw := range tables {
		table, _ := raw.(map[string]any)
		name, _ := table["table"].(string)
		if strings.TrimSpace(name) == "" {
			return briefAuthority{}, errors.New("invalid operating brief catalog table")
		}
		for _, role := range stringSlice(table["roles"]) {
			switch role {
			case "store_dimension":
				if authority.storeTable != "" {
					return briefAuthority{}, errors.New("operating brief store role is ambiguous")
				}
				authority.storeTable = name
			case "operating_week_readiness":
				if authority.readinessTable != "" {
					return briefAuthority{}, errors.New("operating brief readiness role is ambiguous")
				}
				authority.readinessTable = name
			}
		}
	}
	if authority.storeTable == "" || authority.readinessTable == "" {
		return briefAuthority{}, errors.New("operating brief catalog roles unavailable")
	}
	storeDefinition, err := client.TableDefinition(ctx, authority.storeTable)
	if err != nil {
		return briefAuthority{}, err
	}
	readinessDefinition, err := client.TableDefinition(ctx, authority.readinessTable)
	if err != nil {
		return briefAuthority{}, err
	}
	store, _ := storeDefinition["definition"].(map[string]any)
	readiness, _ := readinessDefinition["definition"].(map[string]any)
	if store == nil || readiness == nil || !hasBriefRole(store, "store_dimension") || !hasBriefRole(readiness, "operating_week_readiness") {
		return briefAuthority{}, errors.New("operating brief table definitions changed roles")
	}
	authority.storeDefinition, authority.readinessDefinition = storeDefinition, readinessDefinition
	return authority, nil
}

func hasBriefRole(definition map[string]any, expected string) bool {
	for _, role := range stringSlice(definition["roles"]) {
		if role == expected {
			return true
		}
	}
	return false
}

func briefSQLIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' || (index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func briefCatalogBundle(index, storeDefinition, readinessDefinition map[string]any) (map[string]any, error) {
	source, _ := index["source"].(map[string]any)
	if source == nil {
		return nil, errors.New("invalid operating brief catalog")
	}
	tables := make([]any, 0, 2)
	for _, definition := range []map[string]any{storeDefinition, readinessDefinition} {
		raw, _ := definition["definition"].(map[string]any)
		if raw == nil {
			return nil, errors.New("invalid operating brief table definition")
		}
		tables = append(tables, raw)
	}
	governance, _ := readinessDefinition["governance"].(map[string]any)
	return map[string]any{
		"availability": map[string]any{"status": "available"},
		"governance":   governance,
		"sources": []any{map[string]any{
			"source_id": source["source_id"], "database": source["database"], "engine": source["engine"],
			"semantic_pack": source["semantic_pack"], "status": "ready", "tables": tables,
		}},
	}, nil
}

func briefQueryRows(ctx context.Context, client *agenttools.GovernedDataClient, baseSQL, runID string) ([]map[string]any, error) {
	rows := make([]map[string]any, 0)
	for offset := 0; offset < operatingBriefMaxWeekRows; offset += operatingBriefQueryLimit {
		pageSQL := baseSQL + fmt.Sprintf(" LIMIT %d OFFSET %d", operatingBriefQueryLimit, offset)
		result, err := client.Query(ctx, pageSQL, operatingBriefQueryLimit, runID)
		if err != nil {
			return nil, err
		}
		page := mapSlice(result["rows"])
		if page == nil {
			return nil, errors.New("invalid governed query rows")
		}
		query, _ := result["query"].(map[string]any)
		truncated, _ := query["truncated"].(bool)
		if truncated && len(page) != operatingBriefQueryLimit {
			return nil, errors.New("invalid governed query truncation")
		}
		rows = append(rows, page...)
		if len(page) < operatingBriefQueryLimit {
			return rows, nil
		}
	}
	return nil, errors.New("operating brief dataset reached the 50000 row boundary")
}

func (s *OperatingBriefService) persistProjection(ctx context.Context, payload types.OperatingBriefRefreshPayload, connection types.GovernedEdgeConnection, catalogIndex, result map[string]any) error {
	if result["status"] != "ok" {
		return errors.New("operating brief computation is unavailable")
	}
	projection, _ := result["projection"].(map[string]any)
	if projection == nil {
		return errors.New("operating brief projection unavailable")
	}
	state, _ := projection["state"].(string)
	if state != "ready" && state != "no_data" {
		return errors.New("operating brief projection state is not persistable")
	}
	data, dataOK := projection["data"].(map[string]any)
	questions, questionsOK := projection["handoffQuestions"].(map[string]any)
	if !dataOK || !questionsOK {
		return errors.New("invalid operating brief projection")
	}
	data, rewrittenQuestions, err := rewriteBriefProjection(data, questions)
	if err != nil {
		return err
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}
	questionsJSON, err := json.Marshal(rewrittenQuestions)
	if err != nil {
		return err
	}
	metadataJSON, err := json.Marshal(result["snapshotMetadata"])
	if err != nil {
		return err
	}
	fixedSlotsJSON, err := json.Marshal(result["fixedSlots"])
	if err != nil {
		return err
	}
	catalog, _ := catalogIndex["catalog"].(map[string]any)
	if catalog == nil || stringValue(catalog["version"]) == "" || stringValue(catalog["freshness_token"]) == "" {
		return errors.New("invalid operating brief catalog fence")
	}
	quality, _ := projection["quality"].(string)
	reason, _ := projection["reasonCode"].(string)
	digest, _ := result["inputSetDigest"].(string)
	revision, readyAt := projectionRevisionAndReadyAt(result, data)
	now := s.now().UTC()
	snapshot := &types.OperatingBriefSnapshot{
		SnapshotRef: uuid.NewString(), TenantID: payload.TenantID, ScopeKind: payload.Scope.Kind, StoreID: payload.Scope.StoreID,
		SourceID: connection.SourceID, BindingID: connection.BindingID, BindingRevision: connection.Revision,
		DeploymentRevision: connection.DeploymentRevision, CatalogVersion: stringValue(catalog["version"]), FreshnessToken: stringValue(catalog["freshness_token"]),
		State: state, Quality: quality, ReasonCode: reason, InputSetDigest: digest, Revision: revision, ReadyAt: readyAt,
		Data: types.JSON(dataJSON), HandoffQuestions: types.JSON(questionsJSON), SnapshotMetadata: types.JSON(metadataJSON), FixedSlots: types.JSON(fixedSlotsJSON), GeneratedAt: now,
	}
	labels := map[string]string{}
	if effective, ok := result["effectiveScope"].(map[string]any); ok {
		for k, v := range stringMap(effective["storeLabels"]) {
			labels[k] = v
		}
	}
	if err := s.authorizeBriefConnection(ctx, payload.TenantID, connection); err != nil {
		return err
	}
	return s.repo.SaveSnapshot(ctx, payload.RefreshID, snapshot, labels)
}

func rewriteBriefProjection(data map[string]any, questions map[string]any) (map[string]any, map[string]string, error) {
	dataAnchors, err := collectBriefAnchors(data)
	if err != nil {
		return nil, nil, err
	}
	if len(dataAnchors) != len(questions) {
		return nil, nil, errors.New("operating brief projection anchors do not match questions")
	}
	rewrittenQuestions := make(map[string]string, len(questions))
	anchorRefs := make(map[string]string, len(questions))
	for key, raw := range questions {
		question, ok := raw.(string)
		if !ok || strings.TrimSpace(question) == "" {
			return nil, nil, errors.New("invalid operating brief handoff question")
		}
		if _, ok := dataAnchors[key]; !ok {
			return nil, nil, errors.New("operating brief projection anchors do not match questions")
		}
		anchorRefs[key] = uuid.NewString()
		rewrittenQuestions[anchorRefs[key]] = question
	}
	rewritten, err := rewriteBriefAnchors(data, anchorRefs)
	if err != nil {
		return nil, nil, err
	}
	rewrittenData, ok := rewritten.(map[string]any)
	if !ok {
		return nil, nil, errors.New("invalid operating brief projection data")
	}
	return rewrittenData, rewrittenQuestions, nil
}

func collectBriefAnchors(value any) (map[string]struct{}, error) {
	anchors := map[string]struct{}{}
	var walk func(any) error
	walk = func(current any) error {
		switch typed := current.(type) {
		case map[string]any:
			for key, item := range typed {
				if key == "observationAnchorRef" {
					if item == nil {
						continue
					}
					ref, ok := item.(string)
					if !ok || ref == "" {
						return errors.New("invalid operating brief observation anchor")
					}
					anchors[ref] = struct{}{}
					continue
				}
				if err := walk(item); err != nil {
					return err
				}
			}
		case []any:
			for _, item := range typed {
				if err := walk(item); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return anchors, walk(value)
}

func rewriteBriefAnchors(value any, refs map[string]string) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if key == "observationAnchorRef" {
				if item == nil {
					result[key] = nil
					continue
				}
				raw, ok := item.(string)
				ref, found := refs[raw]
				if !ok || !found {
					return nil, errors.New("operating brief projection anchor is unmapped")
				}
				result[key] = ref
				continue
			}
			rewritten, err := rewriteBriefAnchors(item, refs)
			if err != nil {
				return nil, err
			}
			result[key] = rewritten
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			rewritten, err := rewriteBriefAnchors(item, refs)
			if err != nil {
				return nil, err
			}
			result[i] = rewritten
		}
		return result, nil
	default:
		return value, nil
	}
}

func projectionRevisionAndReadyAt(result map[string]any, data map[string]any) (*int64, string) {
	periods, _ := data["periods"].(map[string]any)
	current, _ := periods["current"].(map[string]any)
	readyAt, _ := current["readyAt"].(string)
	computation, _ := result["computation"].(map[string]any)
	computedCurrent, _ := computation["current"].(map[string]any)
	revision := int64Value(computedCurrent["revision"])
	return revision, readyAt
}

func (s *OperatingBriefService) resolveScope(ctx context.Context, tenantID uint64, ref string) (types.OperatingBriefScope, string, error) {
	if strings.TrimSpace(ref) == "" {
		return types.OperatingBriefScope{Kind: types.OperatingBriefScopeAll}, "所有在营门店", nil
	}
	stored, err := s.repo.ResolveScope(ctx, tenantID, strings.TrimSpace(ref))
	if err != nil {
		return types.OperatingBriefScope{}, "", err
	}
	if stored == nil {
		return types.OperatingBriefScope{}, "", apprepo.ErrOperatingBriefSnapshotNotFound
	}
	return types.OperatingBriefScope{Kind: types.OperatingBriefScopeStore, StoreID: stored.StoreID}, stored.Label, nil
}

func (s *OperatingBriefService) publicSnapshot(ctx context.Context, snapshot *types.OperatingBriefSnapshot, scopeRef, label string, refresh *types.OperatingBriefRefresh) (map[string]any, error) {
	var data map[string]any
	if err := json.Unmarshal(snapshot.Data, &data); err != nil {
		return nil, err
	}
	scopes, err := s.repo.ListScopes(ctx, snapshot.TenantID)
	if err != nil {
		return nil, err
	}
	options := []any{map[string]any{"kind": "all_operating_stores", "label": "所有在营门店", "scopeRef": nil}}
	for _, scope := range scopes {
		options = append(options, map[string]any{"kind": "store", "label": scope.Label, "scopeRef": scope.ScopeRef})
	}
	display := snapshot.State
	if snapshot.State == "ready" && snapshot.Quality == "partial" {
		display = "partial"
	}
	pollable := briefRefreshActive(refresh)
	snapshotRef := any(snapshot.SnapshotRef)
	if snapshot.State != "ready" {
		snapshotRef = nil
	}
	return map[string]any{
		"contractVersion": "operating-brief/2", "displayState": display, "generatedAt": snapshot.GeneratedAt.Format(time.RFC3339Nano), "referenceExpiresAt": nil,
		"selectedScope": map[string]any{"kind": publicScopeKind(snapshot.ScopeKind), "label": label, "scopeRef": nilIfEmpty(scopeRef)}, "scopeOptions": options,
		"weeklyCore": map[string]any{"status": snapshot.State, "completeness": nilIfEmpty(snapshot.Quality), "reasonCode": nilIfEmpty(snapshot.ReasonCode), "pollable": pollable, "retryAfterSeconds": func() any {
			if pollable {
				return operatingBriefRetrySeconds
			}
			return nil
		}(), "inputSetDigest": nilIfEmpty(snapshot.InputSetDigest), "revision": snapshot.Revision, "readyAt": nilIfEmpty(snapshot.ReadyAt), "data": data},
		"inventory": inventoryUnavailable(), "notices": []any{}, "briefSnapshotRef": snapshotRef,
	}, nil
}

func (s *OperatingBriefService) emptyPublic(scopeRef, label string, refresh *types.OperatingBriefRefresh) map[string]any {
	state, reason, pollable := "preparing", "refresh_queued", true
	if refresh != nil && refresh.Status == types.OperatingBriefRefreshFailed {
		state, reason, pollable = "unavailable", "refresh_failed", false
	}
	return map[string]any{
		"contractVersion": "operating-brief/2", "displayState": state, "generatedAt": s.now().UTC().Format(time.RFC3339Nano), "referenceExpiresAt": nil,
		"selectedScope": map[string]any{"kind": publicScopeKind(scopeKindFromRef(scopeRef)), "label": label, "scopeRef": nilIfEmpty(scopeRef)},
		"scopeOptions":  []any{map[string]any{"kind": "all_operating_stores", "label": "所有在营门店", "scopeRef": nil}},
		"weeklyCore": map[string]any{"status": state, "completeness": nil, "reasonCode": reason, "pollable": pollable, "retryAfterSeconds": func() any {
			if pollable {
				return operatingBriefRetrySeconds
			}
			return nil
		}(), "inputSetDigest": nil, "revision": nil, "readyAt": nil, "data": nil},
		"inventory": inventoryUnavailable(), "notices": []any{}, "briefSnapshotRef": nil,
	}
}

func inventoryUnavailable() map[string]any {
	return map[string]any{"status": "not_applicable", "reasonCode": "capability_not_available", "pollable": false, "observations": []any{}}
}
func publicScopeKind(kind string) string {
	if kind == types.OperatingBriefScopeStore {
		return "store"
	}
	return "all_operating_stores"
}
func scopeKindFromRef(ref string) string {
	if ref != "" {
		return types.OperatingBriefScopeStore
	}
	return types.OperatingBriefScopeAll
}
func nilIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func stringValue(value any) string { result, _ := value.(string); return result }
func int64Value(value any) *int64 {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return &parsed
		}
	case float64:
		parsed := int64(typed)
		if float64(parsed) == typed {
			return &parsed
		}
	case int64:
		return &typed
	case int:
		parsed := int64(typed)
		return &parsed
	}
	return nil
}
func anySlice(value any) []any { result, _ := value.([]any); return result }
func stringSlice(value any) []string {
	raw := anySlice(value)
	result := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			result = append(result, s)
		}
	}
	return result
}
func mapSlice(value any) []map[string]any {
	raw, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]map[string]any); ok {
			return typed
		}
		return nil
	}
	result := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		result = append(result, m)
	}
	return result
}
func stringMap(value any) map[string]string {
	raw, _ := value.(map[string]any)
	result := map[string]string{}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}
