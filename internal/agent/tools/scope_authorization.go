package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// LiveKnowledgeAccessTool keeps a turn's server-selected target set from
// becoming a durable authorization cache. It is intentionally a narrow
// decorator: a target revoked after engine construction rejects the entire
// tool call rather than letting any direct SQL/wiki tool inspect stale scope.
type LiveKnowledgeAccessTool struct {
	delegate               types.Tool
	targets                types.SearchTargets
	kbService              interfaces.KnowledgeBaseService
	memberships            interfaces.TenantMemberService
	requiresKnowledgeWrite bool
}

func NewLiveKnowledgeAccessTool(
	delegate types.Tool,
	targets types.SearchTargets,
	kbService interfaces.KnowledgeBaseService,
	memberships interfaces.TenantMemberService,
	requiresKnowledgeWrite bool,
) types.Tool {
	return &LiveKnowledgeAccessTool{
		delegate: delegate, targets: targets, kbService: kbService,
		memberships: memberships, requiresKnowledgeWrite: requiresKnowledgeWrite,
	}
}

func (t *LiveKnowledgeAccessTool) Name() string                { return t.delegate.Name() }
func (t *LiveKnowledgeAccessTool) Description() string         { return t.delegate.Description() }
func (t *LiveKnowledgeAccessTool) Parameters() json.RawMessage { return t.delegate.Parameters() }
func (t *LiveKnowledgeAccessTool) Execute(ctx context.Context, args json.RawMessage) (*types.ToolResult, error) {
	if err := AuthorizeLiveKnowledgeTargets(ctx, t.targets, t.kbService); err != nil {
		return &types.ToolResult{Success: false, Error: "knowledge access is no longer available"}, nil
	}
	if t.requiresKnowledgeWrite && !t.hasLiveKnowledgeWriteAuthority(ctx) {
		return &types.ToolResult{Success: false, Error: "knowledge write access is no longer available"}, nil
	}
	return t.delegate.Execute(ctx, args)
}

// AuthorizeLiveKnowledgeTargets rechecks same-tenant Business Role governance
// at the actual knowledge-read boundary. Cross-tenant targets must carry the
// exact server-issued provenance from the route that already authorized them;
// this layer deliberately does not implement a second share interpreter.
func AuthorizeLiveKnowledgeTargets(
	ctx context.Context,
	targets types.SearchTargets,
	kbService interfaces.KnowledgeBaseService,
) error {
	if kbService == nil {
		return fmt.Errorf("knowledge access cannot be verified")
	}
	checked := false
	for _, target := range targets {
		if target == nil || target.KnowledgeBaseID == "" {
			continue
		}
		checked = true
		if !liveKnowledgeTargetAllowed(ctx, target, kbService) {
			return fmt.Errorf("knowledge access is no longer available")
		}
	}
	if !checked {
		return fmt.Errorf("knowledge access is no longer available")
	}
	return nil
}

// liveKnowledgeTargetAllowed accepts foreign content only through exact
// server-owned provenance. Same-tenant content is always checked through the
// governed KB read so disabling a Business Role takes effect immediately.
func liveKnowledgeTargetAllowed(
	ctx context.Context,
	target *types.SearchTarget,
	kbService interfaces.KnowledgeBaseService,
) bool {
	if target == nil || target.KnowledgeBaseID == "" || kbService == nil {
		return false
	}
	_, directSourceTenantID, directKnowledgeBaseID, _, hasDirectShareProvenance :=
		types.AuthorizedSharedKnowledgeBaseFromContext(ctx)
	if hasDirectShareProvenance {
		return target.TenantID == directSourceTenantID &&
			target.KnowledgeBaseID == directKnowledgeBaseID
	}
	_, provenanceSourceTenantID, _, hasSharedAgentProvenance :=
		types.AuthorizedSharedAgentExecutionFromContext(ctx)
	if hasSharedAgentProvenance {
		return target.TenantID == provenanceSourceTenantID
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 || (target.TenantID != 0 && target.TenantID != tenantID) {
		return false
	}
	kb, err := kbService.GetKnowledgeBaseByID(ctx, target.KnowledgeBaseID)
	return err == nil && kb != nil && kb.TenantID == tenantID
}

func liveCurrentTenantRole(
	ctx context.Context, tenantID uint64, memberships interfaces.TenantMemberService,
) (types.TenantRole, bool) {
	if memberships == nil {
		return "", false
	}
	userID, ok := types.UserIDFromContext(ctx)
	if !ok || userID == "" || types.IsSyntheticUserID(userID) {
		return "", false
	}
	membership, err := memberships.GetMembership(ctx, userID, tenantID)
	if err != nil || membership == nil || membership.DeletedAt.Valid ||
		membership.UserID != userID || membership.TenantID != tenantID ||
		membership.Status != types.TenantMemberStatusActive {
		return "", false
	}
	return membership.Role, true
}

func (t *LiveKnowledgeAccessTool) hasLiveKnowledgeWriteAuthority(ctx context.Context) bool {
	if scope, isAPIKey := types.TenantAPIKeyScopeFromContext(ctx); isAPIKey {
		return scope.FullAccess ||
			scope.HasCapability(types.APIKeyCapabilityManageKnowledgeBases) ||
			scope.HasCapability(types.APIKeyCapabilityIngest)
	}
	if _, _, _, sharedAgent := types.AuthorizedSharedAgentExecutionFromContext(ctx); sharedAgent {
		return false
	}
	if _, _, _, permission, directShare := types.AuthorizedSharedKnowledgeBaseFromContext(ctx); directShare {
		return permission.HasPermission(types.OrgRoleEditor)
	}
	callerTenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || callerTenantID == 0 {
		return false
	}
	role, ok := liveCurrentTenantRole(ctx, callerTenantID, t.memberships)
	if !ok {
		return false
	}
	return role.HasPermission(types.TenantRoleAdmin)
}

func effectiveSearchTargetTagIDs(target *types.SearchTarget) []string {
	if target == nil {
		return nil
	}
	return dedupNonEmptyStrings(append(
		append([]string(nil), target.TagIDs...), target.ScopeTagIDs...,
	))
}

// searchTargetScope returns what a SINGLE search target authorizes.
//
// Inside one target, KnowledgeIDs and tags are an intersection, never a union.
// A tag-scoped mention is built by resolving the tag relation table into
// KnowledgeIDs and intersecting that with any explicitly mentioned documents;
// TagIDs/ScopeTagIDs are kept alongside as the physical index filter and as
// the logical scope record. Treating them as an independent way to authorize a
// document would re-admit every document carrying the tag and silently undo
// that intersection. Tags therefore only authorize when the target carries no
// resolved document whitelist.
//
// Alternatives ACROSS targets remain a union; that merge happens in callers.
func searchTargetScope(target *types.SearchTarget) (knowledgeIDs, tagIDs []string) {
	if target == nil {
		return nil, nil
	}
	knowledgeIDs = dedupNonEmptyStrings(target.KnowledgeIDs)
	if len(knowledgeIDs) > 0 {
		return knowledgeIDs, nil
	}
	return nil, effectiveSearchTargetTagIDs(target)
}

// searchTargetIsWholeKB reports whether a target grants unrestricted access to
// its knowledge base.
func searchTargetIsWholeKB(target *types.SearchTarget) bool {
	if target == nil {
		return false
	}
	knowledgeIDs, tagIDs := searchTargetScope(target)
	return target.Type == types.SearchTargetTypeKnowledgeBase &&
		len(knowledgeIDs) == 0 && len(tagIDs) == 0
}

// authorizeKnowledgeInSearchTargets is the shared authorization boundary for
// every Agent tool that accepts a model-visible dN/knowledge_id. Handle
// decoding is necessary but never sufficient: the durable document must also
// belong to the server-owned search scope for this Agent execution.
func authorizeKnowledgeInSearchTargets(
	ctx context.Context,
	searchTargets types.SearchTargets,
	knowledgeID string,
	knowledgeService interfaces.KnowledgeService,
) (*types.Knowledge, error) {
	knowledgeID = strings.TrimSpace(knowledgeID)
	if knowledgeID == "" {
		return nil, fmt.Errorf("knowledge_id is required")
	}
	if knowledgeService == nil {
		return nil, fmt.Errorf("knowledge service is unavailable")
	}
	knowledge, err := knowledgeService.GetKnowledgeByIDOnly(ctx, knowledgeID)
	if err != nil || knowledge == nil {
		if err == nil {
			err = fmt.Errorf("empty result")
		}
		return nil, fmt.Errorf("document %s not found: %w", knowledgeID, err)
	}
	// A same-tenant Agent turn must consult the live knowledge service, not
	// only the target snapshot captured when the turn began. That service is
	// backed by KnowledgeAccess and therefore observes a role being disabled
	// while an Agent session is still open. Foreign shared KBs keep their
	// pre-existing organization-share boundary.
	_, sharedSourceTenantID, _, sharedAgent := types.AuthorizedSharedAgentExecutionFromContext(ctx)
	if tenantID, ok := types.TenantIDFromContext(ctx); ok && tenantID == knowledge.TenantID &&
		(!sharedAgent || knowledge.TenantID != sharedSourceTenantID) {
		knowledge, err = knowledgeService.GetKnowledgeByID(ctx, knowledgeID)
		if err != nil || knowledge == nil {
			if err == nil {
				err = fmt.Errorf("empty result")
			}
			return nil, fmt.Errorf("document %s is not accessible: %w", knowledgeID, err)
		}
	}
	if !searchTargets.ContainsKB(knowledge.KnowledgeBaseID) {
		return nil, fmt.Errorf("knowledge base %s is not within the current Agent scope", knowledge.KnowledgeBaseID)
	}
	allowed, err := searchTargetsAllowKnowledgeID(
		ctx, searchTargets, knowledge.ID, knowledge.KnowledgeBaseID, knowledgeService,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to validate document scope: %w", err)
	}
	if !allowed {
		return nil, fmt.Errorf("document %s is not within the current @mention scope", knowledge.ID)
	}
	return knowledge, nil
}

// authorizeChunkInSearchTargets is the chunk/FAQ counterpart of
// authorizeKnowledgeInSearchTargets. A chunk ID is accepted only after the
// server resolves its owning document and validates that document against the
// full KB/document/tag scope.
func authorizeChunkInSearchTargets(
	ctx context.Context,
	searchTargets types.SearchTargets,
	chunkID string,
	chunkService interfaces.ChunkService,
	knowledgeService interfaces.KnowledgeService,
) (*types.Chunk, error) {
	chunkID = strings.TrimSpace(chunkID)
	if chunkID == "" {
		return nil, fmt.Errorf("chunk_id is required")
	}
	if chunkService == nil {
		return nil, fmt.Errorf("chunk service is unavailable")
	}
	chunk, err := chunkService.GetChunkByIDOnly(ctx, chunkID)
	if err != nil || chunk == nil {
		if err == nil {
			err = fmt.Errorf("empty result")
		}
		return nil, fmt.Errorf("chunk %s not found: %w", chunkID, err)
	}
	if !chunk.IsEnabled {
		return nil, fmt.Errorf("chunk %s is disabled", chunk.ID)
	}
	if !searchTargets.ContainsKB(chunk.KnowledgeBaseID) {
		return nil, fmt.Errorf("knowledge base %s is not within the current Agent scope", chunk.KnowledgeBaseID)
	}
	allowed, err := searchTargetsAllowKnowledgeID(
		ctx, searchTargets, chunk.KnowledgeID, chunk.KnowledgeBaseID, knowledgeService,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to validate chunk scope: %w", err)
	}
	if !allowed {
		return nil, fmt.Errorf("chunk %s is not within the current @mention scope", chunk.ID)
	}
	return chunk, nil
}

// validateKnowledgeBaseIDsInSearchTargets rejects hallucinated, stale, or
// out-of-scope bN values after the model-context registry resolves them.
func validateKnowledgeBaseIDsInSearchTargets(searchTargets types.SearchTargets, kbIDs []string) error {
	for _, kbID := range dedupNonEmptyStrings(kbIDs) {
		if !searchTargets.ContainsKB(kbID) {
			return fmt.Errorf("knowledge base %s is not within the current Agent scope", kbID)
		}
	}
	return nil
}

// resolveAuthorizedSourceRefs validates Wiki source_refs/suspected IDs against
// the same Agent scope and rebuilds the stored "uuid|title" representation
// from server data instead of trusting a model-supplied title suffix.
func resolveAuthorizedSourceRefs(
	ctx context.Context,
	searchTargets types.SearchTargets,
	refs []string,
	knowledgeService interfaces.KnowledgeService,
) ([]string, error) {
	resolved := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		knowledgeID := strings.TrimSpace(strings.SplitN(ref, "|", 2)[0])
		if knowledgeID == "" {
			continue
		}
		knowledge, err := authorizeKnowledgeInSearchTargets(ctx, searchTargets, knowledgeID, knowledgeService)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[knowledge.ID]; exists {
			continue
		}
		seen[knowledge.ID] = struct{}{}
		title := strings.TrimSpace(knowledge.Title)
		if title == "" {
			title = strings.TrimSpace(knowledge.FileName)
		}
		if title == "" {
			resolved = append(resolved, knowledge.ID)
		} else {
			resolved = append(resolved, knowledge.ID+"|"+title)
		}
	}
	return resolved, nil
}

type knowledgeTagsFetcher func(context.Context, []string) (map[string][]*types.KnowledgeTag, error)

func searchTargetsAllowKnowledgeID(
	ctx context.Context,
	searchTargets types.SearchTargets,
	knowledgeID string,
	kbID string,
	knowledgeService interfaces.KnowledgeService,
) (bool, error) {
	if knowledgeID == "" || kbID == "" {
		return false, nil
	}

	var tagIDs []string
	matchedKB := false
	for _, target := range searchTargets {
		if target == nil || target.KnowledgeBaseID != kbID {
			continue
		}
		matchedKB = true
		if searchTargetIsWholeKB(target) {
			return true, nil
		}
		targetKnowledgeIDs, targetTagIDs := searchTargetScope(target)
		for _, allowedID := range targetKnowledgeIDs {
			if allowedID == knowledgeID {
				return true, nil
			}
		}
		tagIDs = append(tagIDs, targetTagIDs...)
	}
	if !matchedKB || len(tagIDs) == 0 || knowledgeService == nil {
		return false, nil
	}

	matches, err := knowledgeIDsMatchingAnyTag(ctx, []string{knowledgeID}, tagIDs, knowledgeService.GetKnowledgeTags)
	if err != nil {
		return false, err
	}
	return matches[knowledgeID], nil
}

// filterSearchResultsInSearchTargets applies the same whole-KB/document/tag
// union semantics to tools whose backend can only query by KB (notably the
// knowledge graph). It batches tag lookup and rejects results without enough
// provenance instead of turning a narrow mention into whole-KB access.
func filterSearchResultsInSearchTargets(
	ctx context.Context,
	searchTargets types.SearchTargets,
	kbID string,
	results []*types.SearchResult,
	knowledgeService interfaces.KnowledgeService,
) ([]*types.SearchResult, error) {
	var explicitIDs []string
	var tagIDs []string
	matchedKB := false
	for _, target := range searchTargets {
		if target == nil || target.KnowledgeBaseID != kbID {
			continue
		}
		matchedKB = true
		if searchTargetIsWholeKB(target) {
			return results, nil
		}
		targetKnowledgeIDs, targetTagIDs := searchTargetScope(target)
		explicitIDs = append(explicitIDs, targetKnowledgeIDs...)
		tagIDs = append(tagIDs, targetTagIDs...)
	}
	if !matchedKB {
		return nil, fmt.Errorf("knowledge base %s is not within the current Agent scope", kbID)
	}

	explicitSet := make(map[string]struct{}, len(explicitIDs))
	for _, id := range dedupNonEmptyStrings(explicitIDs) {
		explicitSet[id] = struct{}{}
	}
	remainingIDs := make([]string, 0, len(results))
	for _, result := range results {
		if result == nil || result.KnowledgeID == "" {
			continue
		}
		if result.KnowledgeBaseID != "" && result.KnowledgeBaseID != kbID {
			return nil, fmt.Errorf(
				"graph result document %s belongs to knowledge base %s, expected %s",
				result.KnowledgeID, result.KnowledgeBaseID, kbID,
			)
		}
		if _, ok := explicitSet[result.KnowledgeID]; !ok {
			remainingIDs = append(remainingIDs, result.KnowledgeID)
		}
	}
	var tagMatches map[string]bool
	if len(tagIDs) > 0 {
		if knowledgeService == nil {
			return nil, fmt.Errorf("knowledge service is unavailable for tag-scoped graph filtering")
		}
		var err error
		tagMatches, err = knowledgeIDsMatchingAnyTag(
			ctx, remainingIDs, tagIDs, knowledgeService.GetKnowledgeTags,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to validate graph result scope: %w", err)
		}
	}

	filtered := make([]*types.SearchResult, 0, len(results))
	for _, result := range results {
		if result == nil || result.KnowledgeID == "" {
			continue
		}
		_, explicit := explicitSet[result.KnowledgeID]
		if explicit || tagMatches[result.KnowledgeID] {
			filtered = append(filtered, result)
		}
	}
	return filtered, nil
}

func knowledgeIDsMatchingAnyTag(
	ctx context.Context,
	knowledgeIDs []string,
	tagIDs []string,
	fetchTags knowledgeTagsFetcher,
) (map[string]bool, error) {
	result := make(map[string]bool)
	if len(knowledgeIDs) == 0 || len(tagIDs) == 0 || fetchTags == nil {
		return result, nil
	}

	uniqueKnowledgeIDs := dedupNonEmptyStrings(knowledgeIDs)
	uniqueTagIDs := dedupNonEmptyStrings(tagIDs)
	if len(uniqueKnowledgeIDs) == 0 || len(uniqueTagIDs) == 0 {
		return result, nil
	}

	tagSet := make(map[string]bool, len(uniqueTagIDs))
	for _, tagID := range uniqueTagIDs {
		tagSet[tagID] = true
	}

	tagMap, err := fetchTags(ctx, uniqueKnowledgeIDs)
	if err != nil {
		return nil, err
	}
	for _, knowledgeID := range uniqueKnowledgeIDs {
		for _, tag := range tagMap[knowledgeID] {
			if tag != nil && tagSet[tag.ID] {
				result[knowledgeID] = true
				break
			}
		}
	}
	return result, nil
}
