package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

var ErrInvalidCapabilityConfiguration = errors.New("invalid capability configuration")

// CreateCapabilityPlanInput is the strict body accepted by the version registry.
type CreateCapabilityPlanInput struct {
	ServiceLevel   string                     `json:"service_level"`
	CapabilityRefs types.AICapabilityPlanRefs `json:"capability_refs"`
}

// CapabilityPlanList is the management projection of the version registry.
type CapabilityPlanList struct {
	Items            []types.AICapabilityPlanDocument `json:"items"`
	DefaultVersionID *string                          `json:"default_version_id"`
}

// TenantCapabilityPlan is the effective management projection for one tenant.
type TenantCapabilityPlan struct {
	Source string                         `json:"source"`
	Plan   types.AICapabilityPlanDocument `json:"plan"`
}

// CapabilityPlanService resolves runtime configuration from the local database
// and exposes the system-admin management operations over the same authority.
type CapabilityPlanService struct {
	repo repository.CapabilityPlanRepository
	now  func() time.Time
}

// ResolveActivationPlanVersion returns the current default version to pin into
// a new activation command. Exact replays use the version on the receipt.
func (s *CapabilityPlanService) ResolveActivationPlanVersion(ctx context.Context, actorID string) (string, error) {
	_, defaultID, err := s.repo.ListPlans(ctx, actorID)
	if err != nil {
		return "", err
	}
	if defaultID == nil || !validRequiredText(*defaultID) {
		return "", interfaces.ErrAICapabilityUnavailable
	}
	return *defaultID, nil
}

// NewCapabilityPlanService constructs the single local capability authority.
func NewCapabilityPlanService(repo repository.CapabilityPlanRepository) *CapabilityPlanService {
	return &CapabilityPlanService{repo: repo, now: time.Now}
}

func validRequiredText(value string) bool {
	return value != "" && value == strings.TrimSpace(value)
}

func validStoredPlan(plan *types.AICapabilityPlanVersion) bool {
	return plan != nil && plan.ContractVersion == types.AICapabilityPlanContractVersion &&
		validRequiredText(plan.VersionID) && validRequiredText(plan.ServiceLevel) &&
		validRequiredText(plan.EmployeeAssistantRequestRuntimeRef) &&
		validRequiredText(plan.OperatingAnalysisRequestRuntimeRef) &&
		validRequiredText(plan.EmbeddingRef) && validRequiredText(plan.RerankingRef) &&
		validRequiredText(plan.ParsingRef)
}

func validSource(source string) bool {
	return source == "tenant_assignment" || source == "platform_default"
}

func (s *CapabilityPlanService) resolvePlan(ctx context.Context, tenantID uint64) (*types.ResolvedAICapabilityPlan, error) {
	if tenantID == 0 {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	resolved, err := s.repo.ResolvePlan(ctx, tenantID)
	if err != nil || resolved == nil || !validSource(resolved.Source) || !validStoredPlan(&resolved.Plan) {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	return resolved, nil
}

// Resolve returns the frozen enterprise capability projection.
func (s *CapabilityPlanService) Resolve(
	ctx context.Context, tenantID uint64,
) (*types.AICapabilityPlanResolution, error) {
	resolved, err := s.resolvePlan(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &types.AICapabilityPlanResolution{
		ContractVersion: types.AICapabilityPlanContractVersion,
		PlanVersionID:   resolved.Plan.VersionID,
		Source:          resolved.Source,
		Enterprise: types.EnterpriseAICapabilityProjection{
			ServiceLevel: resolved.Plan.ServiceLevel,
			Status:       "active",
			Health:       types.EnterpriseAIHealth{Status: "unknown"},
		},
	}, nil
}

// ResolveKnowledgeProcessingPlan returns the immutable plan pin for new KBs.
func (s *CapabilityPlanService) ResolveKnowledgeProcessingPlan(
	ctx context.Context, tenantID uint64,
) (*types.KnowledgeProcessingPlanPin, error) {
	resolved, err := s.resolvePlan(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &types.KnowledgeProcessingPlanPin{
		ContractVersion: types.KnowledgeProcessingPlanContractVersion,
		PlanVersionID:   resolved.Plan.VersionID,
	}, nil
}

// ResolvePlatformModelRuntimeSettings returns the two opaque runtime references.
func (s *CapabilityPlanService) ResolvePlatformModelRuntimeSettings(
	ctx context.Context, tenantID uint64,
) (*types.PlatformModelRuntimeSettings, error) {
	resolved, err := s.resolvePlan(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	settings := &types.PlatformModelRuntimeSettings{
		ContractVersion: types.PlatformModelRuntimeContractVersion,
		Scope:           types.PlatformSettingsScope{Kind: settingsScopeKind(resolved.Source), ProductBaseTenantID: strconv.FormatUint(tenantID, 10)},
		ActivePlan:      types.PlatformActivePlan{ContractVersion: types.AICapabilityPlanContractVersion, VersionID: resolved.Plan.VersionID},
	}
	settings.RequestRuntimeRefs.EmployeeAssistantRequestRuntime = resolved.Plan.EmployeeAssistantRequestRuntimeRef
	settings.RequestRuntimeRefs.OperatingAnalysisRequestRuntime = resolved.Plan.OperatingAnalysisRequestRuntimeRef
	return settings, nil
}

// ResolvePlatformRetrievalProcessingSettings returns the three opaque processing references.
func (s *CapabilityPlanService) ResolvePlatformRetrievalProcessingSettings(
	ctx context.Context, tenantID uint64,
) (*types.PlatformRetrievalProcessingSettings, error) {
	resolved, err := s.resolvePlan(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	settings := &types.PlatformRetrievalProcessingSettings{
		ContractVersion: types.PlatformRetrievalContractVersion,
		Scope:           types.PlatformSettingsScope{Kind: settingsScopeKind(resolved.Source), ProductBaseTenantID: strconv.FormatUint(tenantID, 10)},
		ActivePlan:      types.PlatformActivePlan{ContractVersion: types.AICapabilityPlanContractVersion, VersionID: resolved.Plan.VersionID},
	}
	settings.CapabilityRefs.Embedding = resolved.Plan.EmbeddingRef
	settings.CapabilityRefs.Reranking = resolved.Plan.RerankingRef
	settings.CapabilityRefs.Parsing = resolved.Plan.ParsingRef
	return settings, nil
}

func settingsScopeKind(source string) string {
	if source == "tenant_assignment" {
		return "enterprise_assigned"
	}
	return "platform_shared"
}

// ResolveAssistantScenarioCapabilities resolves override before default and
// treats an absent default as the valid all-false policy.
func (s *CapabilityPlanService) ResolveAssistantScenarioCapabilities(
	ctx context.Context, tenantID uint64,
) (*types.AssistantScenarioCapabilitySettings, error) {
	if tenantID == 0 {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	resolved, err := s.repo.ResolveScenario(ctx, tenantID)
	if err != nil || resolved == nil || !validSource(resolved.Source) {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	return &types.AssistantScenarioCapabilitySettings{
		ContractVersion: types.AssistantScenarioCapabilityContractVersion,
		Scope:           types.PlatformSettingsScope{Kind: settingsScopeKind(resolved.Source), ProductBaseTenantID: strconv.FormatUint(tenantID, 10)},
		Capabilities:    resolved.Capabilities,
	}, nil
}

// ResolveEnterpriseAdministrationQueue derives the Center-free portion from
// durable tenant facts. Edge nodes stay absent because Platform has no local
// heartbeat observation that could truthfully classify a node online/offline.
func (s *CapabilityPlanService) ResolveEnterpriseAdministrationQueue(
	ctx context.Context,
	tenantID uint64,
	facts types.EnterpriseAdministrationFacts,
) (*types.EnterpriseAdministrationPlatformProjection, error) {
	if tenantID == 0 || facts.ActiveMemberCount < 0 || facts.OperatingAnalysisMissingAccessCount < 0 ||
		facts.OperatingAnalysisMissingAccessCount > facts.ActiveMemberCount ||
		(facts.Role != "admin" && facts.Role != "owner") {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	tenant, err := s.repo.GetTenant(ctx, tenantID)
	if err != nil || tenant == nil {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	resolved, planErr := s.resolvePlan(ctx, tenantID)
	planReady := planErr == nil && resolved != nil
	overSeats := tenant.SeatsTotal != nil && facts.ActiveMemberCount > *tenant.SeatsTotal
	bindingAttention := tenant.GovernedEdgeBinding != nil && !tenant.GovernedEdgeBinding.Enabled
	licenseAttention := tenant.Status != types.TenantStatusActive || !planReady || overSeats || bindingAttention
	items := make([]types.EnterpriseAdministrationItem, 0, 2)
	if licenseAttention {
		items = append(items, types.EnterpriseAdministrationItem{
			Code: "license_or_capability_attention", Priority: "critical", Count: 1, Target: "service_health",
		})
	}
	if !licenseAttention && facts.OperatingAnalysisMissingAccessCount > 0 {
		items = append(items, types.EnterpriseAdministrationItem{
			Code: "operating_analysis_access_gap", Priority: "high",
			Count: facts.OperatingAnalysisMissingAccessCount, Target: "members",
		})
	}
	serviceLevel := "unknown"
	if planReady {
		serviceLevel = resolved.Plan.ServiceLevel
	}
	status, health := "active", "unknown"
	if licenseAttention {
		status, health = "attention", "attention"
	}
	return &types.EnterpriseAdministrationPlatformProjection{
		ContractVersion: types.EnterpriseAdministrationQueueV1,
		Scope:           types.PlatformSettingsScope{Kind: "enterprise_assigned", ProductBaseTenantID: strconv.FormatUint(tenantID, 10)},
		AsOf:            s.now().UTC().Format(time.RFC3339Nano),
		Summary: types.EnterpriseAdministrationPlatformSummary{
			ServiceLevel: serviceLevel, Status: status, MemberQuota: tenant.SeatsTotal, Health: health,
		},
		Items: items,
	}, nil
}

// CreatePlan appends one immutable management version.
func (s *CapabilityPlanService) CreatePlan(
	ctx context.Context, actorID string, input CreateCapabilityPlanInput,
) (*types.AICapabilityPlanDocument, error) {
	refs := input.CapabilityRefs
	if !validRequiredText(input.ServiceLevel) || !validRequiredText(refs.EmployeeAssistantRequestRuntime) ||
		!validRequiredText(refs.OperatingAnalysisRequestRuntime) || !validRequiredText(refs.Embedding) ||
		!validRequiredText(refs.Reranking) || !validRequiredText(refs.Parsing) {
		return nil, ErrInvalidCapabilityConfiguration
	}
	plan := &types.AICapabilityPlanVersion{
		VersionID:                          "plan_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		ContractVersion:                    types.AICapabilityPlanContractVersion,
		ServiceLevel:                       input.ServiceLevel,
		EmployeeAssistantRequestRuntimeRef: refs.EmployeeAssistantRequestRuntime,
		OperatingAnalysisRequestRuntimeRef: refs.OperatingAnalysisRequestRuntime,
		EmbeddingRef:                       refs.Embedding, RerankingRef: refs.Reranking, ParsingRef: refs.Parsing,
		CreatedBy: actorID, CreatedAt: s.now().UTC(),
	}
	if !validStoredPlan(plan) {
		return nil, ErrInvalidCapabilityConfiguration
	}
	if err := s.repo.CreatePlan(ctx, actorID, plan); err != nil {
		return nil, err
	}
	document := plan.Document()
	return &document, nil
}

// ListPlans returns immutable versions and the singleton default pointer.
func (s *CapabilityPlanService) ListPlans(ctx context.Context, actorID string) (*CapabilityPlanList, error) {
	rows, defaultID, err := s.repo.ListPlans(ctx, actorID)
	if err != nil {
		return nil, err
	}
	items := make([]types.AICapabilityPlanDocument, 0, len(rows))
	for i := range rows {
		items = append(items, rows[i].Document())
	}
	return &CapabilityPlanList{Items: items, DefaultVersionID: defaultID}, nil
}

// SetDefaultPlan replaces the singleton default pointer.
func (s *CapabilityPlanService) SetDefaultPlan(ctx context.Context, actorID, versionID string) error {
	if !validRequiredText(versionID) {
		return ErrInvalidCapabilityConfiguration
	}
	return s.repo.SetDefaultPlan(ctx, actorID, versionID, s.now().UTC())
}

// GetTenantPlan returns the effective version and source for management.
func (s *CapabilityPlanService) GetTenantPlan(ctx context.Context, actorID string, tenantID uint64) (*TenantCapabilityPlan, error) {
	if err := s.repo.ValidateSystemAdministrator(ctx, actorID); err != nil {
		return nil, err
	}
	resolved, err := s.resolvePlan(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &TenantCapabilityPlan{Source: resolved.Source, Plan: resolved.Plan.Document()}, nil
}

// AssignTenantPlan sets a tenant override.
func (s *CapabilityPlanService) AssignTenantPlan(ctx context.Context, actorID string, tenantID uint64, versionID string) error {
	if tenantID == 0 || !validRequiredText(versionID) {
		return ErrInvalidCapabilityConfiguration
	}
	return s.repo.AssignTenantPlan(ctx, actorID, tenantID, versionID, s.now().UTC())
}

// ClearTenantPlan restores inheritance from the platform default.
func (s *CapabilityPlanService) ClearTenantPlan(ctx context.Context, actorID string, tenantID uint64) error {
	if tenantID == 0 {
		return ErrInvalidCapabilityConfiguration
	}
	return s.repo.ClearTenantPlan(ctx, actorID, tenantID, s.now().UTC())
}

// GetDefaultScenario returns the valid all-false policy when no row exists.
func (s *CapabilityPlanService) GetDefaultScenario(ctx context.Context, actorID string) (types.AssistantScenarioCapabilities, error) {
	return s.repo.GetDefaultScenario(ctx, actorID)
}

// SetDefaultScenario replaces all three default booleans atomically.
func (s *CapabilityPlanService) SetDefaultScenario(ctx context.Context, actorID string, capabilities types.AssistantScenarioCapabilities) error {
	return s.repo.SetDefaultScenario(ctx, actorID, capabilities, s.now().UTC())
}

// GetTenantScenario returns override before default and preserves explicit false.
func (s *CapabilityPlanService) GetTenantScenario(ctx context.Context, actorID string, tenantID uint64) (*types.ResolvedAssistantScenarioCapabilities, error) {
	if err := s.repo.ValidateSystemAdministrator(ctx, actorID); err != nil {
		return nil, err
	}
	resolved, err := s.repo.ResolveScenario(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		return nil, repository.ErrCapabilityTenantNotFound
	}
	return resolved, nil
}

// SetTenantScenario stores all three override booleans atomically.
func (s *CapabilityPlanService) SetTenantScenario(ctx context.Context, actorID string, tenantID uint64, capabilities types.AssistantScenarioCapabilities) error {
	if tenantID == 0 {
		return ErrInvalidCapabilityConfiguration
	}
	return s.repo.SetTenantScenario(ctx, actorID, tenantID, capabilities, s.now().UTC())
}

// ClearTenantScenario restores inheritance from the platform default.
func (s *CapabilityPlanService) ClearTenantScenario(ctx context.Context, actorID string, tenantID uint64) error {
	if tenantID == 0 {
		return ErrInvalidCapabilityConfiguration
	}
	return s.repo.ClearTenantScenario(ctx, actorID, tenantID, s.now().UTC())
}
