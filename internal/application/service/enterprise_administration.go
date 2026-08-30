package service

import (
	"context"
	"sort"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type enterpriseAdministrationTenantReader interface {
	GetTenantByID(context.Context, uint64) (*types.Tenant, error)
}

type enterpriseAdministrationMemberReader interface {
	ListByTenant(context.Context, uint64) ([]*types.TenantMember, error)
}

type enterpriseAdministrationInvitationReader interface {
	ListByTenant(context.Context, uint64, bool) ([]*types.TenantInvitation, error)
}

type enterpriseAdministrationKBReader interface {
	ListKnowledgeBasesByTenantID(context.Context, uint64) ([]*types.KnowledgeBase, error)
}

type enterpriseAdministrationKnowledgeCounter interface {
	CountKnowledgeByStatus(context.Context, uint64, string, []string) (int64, error)
}

type enterpriseAdministrationAuditReader interface {
	List(context.Context, uint64, *interfaces.AuditLogQuery) ([]*types.AuditLog, error)
}

type enterpriseAdministrationService struct {
	tenant       enterpriseAdministrationTenantReader
	members      enterpriseAdministrationMemberReader
	invitations  enterpriseAdministrationInvitationReader
	knowledge    enterpriseAdministrationKBReader
	knowledgeCnt enterpriseAdministrationKnowledgeCounter
	audit        enterpriseAdministrationAuditReader
	platform     interfaces.EnterpriseAdministrationQueueResolver
}

func NewEnterpriseAdministrationService(
	tenant interfaces.TenantService,
	members interfaces.TenantMemberService,
	invitations interfaces.TenantInvitationService,
	knowledge interfaces.KnowledgeBaseRepository,
	knowledgeCnt interfaces.KnowledgeRepository,
	audit interfaces.AuditLogService,
	platform interfaces.EnterpriseAdministrationQueueResolver,
) interfaces.EnterpriseAdministrationService {
	return &enterpriseAdministrationService{
		tenant:       tenant,
		members:      members,
		invitations:  invitations,
		knowledge:    knowledge,
		knowledgeCnt: knowledgeCnt,
		audit:        audit,
		platform:     platform,
	}
}

func enterpriseAdministrationRole(role types.TenantRole) string {
	switch role {
	case types.TenantRoleOwner:
		return "owner"
	case types.TenantRoleAdmin:
		return "admin"
	case types.TenantRoleContributor:
		return "knowledge_administrator"
	default:
		return ""
	}
}

func (s *enterpriseAdministrationService) Resolve(ctx context.Context) (*types.EnterpriseAdministrationQueue, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	role := enterpriseAdministrationRole(types.TenantRoleFromContext(ctx))
	if role == "" {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	tenant, err := s.tenant.GetTenantByID(ctx, tenantID)
	if err != nil || tenant == nil {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	members, err := s.members.ListByTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	activeMemberCount := 0
	missingAnalysisAccess := 0
	for _, member := range members {
		if member == nil || member.Status != types.TenantMemberStatusActive {
			continue
		}
		activeMemberCount++
		if !member.OperatingAnalysisAccess {
			missingAnalysisAccess++
		}
	}

	facts := types.EnterpriseAdministrationFacts{
		Role:                                role,
		ActiveMemberCount:                   activeMemberCount,
		OperatingAnalysisMissingAccessCount: missingAnalysisAccess,
	}
	platform, platformErr := s.platform.ResolveEnterpriseAdministrationQueue(ctx, tenantID, facts)
	if platformErr != nil || platform == nil {
		platform = unavailableEnterpriseAdministrationPlatform(role)
	}

	items := append([]types.EnterpriseAdministrationItem(nil), platform.Items...)
	if role == "owner" || role == "admin" {
		invitations, listErr := s.invitations.ListByTenant(ctx, tenantID, false)
		if listErr != nil {
			return nil, listErr
		}
		pending := 0
		for _, invitation := range invitations {
			if invitation != nil && invitation.InviteeUserID != "" {
				pending++
			}
		}
		if pending > 0 {
			items = append(items, types.EnterpriseAdministrationItem{
				Code: "pending_invitations", Priority: "medium", Count: pending, Target: "members",
			})
		}
	}

	failedKnowledge, err := s.failedKnowledgeCount(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if failedKnowledge > 0 {
		items = append(items, types.EnterpriseAdministrationItem{
			Code: "knowledge_processing_failed", Priority: "high", Count: failedKnowledge, Target: "knowledge",
		})
	}

	if role == "owner" || role == "admin" {
		riskCount, riskErr := s.recentHighRiskCount(ctx, tenantID)
		if riskErr != nil {
			return nil, riskErr
		}
		if riskCount > 0 {
			items = append(items, types.EnterpriseAdministrationItem{
				Code: "recent_high_risk_operations", Priority: "medium", Count: riskCount, Target: "audit",
			})
		}
	}

	priority := map[string]int{"critical": 0, "high": 1, "medium": 2}
	sort.SliceStable(items, func(i, j int) bool {
		return priority[items[i].Priority] < priority[items[j].Priority]
	})
	return &types.EnterpriseAdministrationQueue{
		ContractVersion: types.EnterpriseAdministrationQueueV1,
		AsOf:            platform.AsOf,
		Summary: types.EnterpriseAdministrationSummary{
			EnterpriseAdministrationPlatformSummary: platform.Summary,
			MemberUsage:                             activeMemberCount,
			StorageUsageByte:                        tenant.StorageUsed,
			StorageQuotaByte:                        tenant.StorageQuota,
		},
		Items: items,
	}, nil
}

func unavailableEnterpriseAdministrationPlatform(role string) *types.EnterpriseAdministrationPlatformProjection {
	items := []types.EnterpriseAdministrationItem{}
	if role == "owner" || role == "admin" {
		items = append(items, types.EnterpriseAdministrationItem{
			Code: "license_or_capability_attention", Priority: "critical", Count: 1, Target: "service_health",
		})
	}
	return &types.EnterpriseAdministrationPlatformProjection{
		ContractVersion: types.EnterpriseAdministrationQueueV1,
		AsOf:            time.Now().UTC().Format(time.RFC3339Nano),
		Summary: types.EnterpriseAdministrationPlatformSummary{
			ServiceLevel: "unknown", Status: "unavailable", Health: "unknown",
		},
		Items: items,
	}
}

func (s *enterpriseAdministrationService) failedKnowledgeCount(ctx context.Context, tenantID uint64) (int, error) {
	knowledgeBases, err := s.knowledge.ListKnowledgeBasesByTenantID(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	total := int64(0)
	for _, knowledgeBase := range knowledgeBases {
		if knowledgeBase == nil {
			continue
		}
		count, countErr := s.knowledgeCnt.CountKnowledgeByStatus(
			ctx, tenantID, knowledgeBase.ID, []string{types.ParseStatusFailed},
		)
		if countErr != nil {
			return 0, countErr
		}
		total += count
	}
	return int(total), nil
}

var enterpriseAdministrationHighRiskActions = map[types.AuditAction]struct{}{
	types.AuditActionMemberRemoved:                  {},
	types.AuditActionMemberStatusChanged:            {},
	types.AuditActionOperatingAnalysisAccessChanged: {},
	types.AuditActionOwnershipTransferred:           {},
}

func (s *enterpriseAdministrationService) recentHighRiskCount(ctx context.Context, tenantID uint64) (int, error) {
	entries, err := s.audit.List(ctx, tenantID, &interfaces.AuditLogQuery{Limit: 100, UnscopedOnly: true})
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	count := 0
	for _, entry := range entries {
		if entry == nil || entry.CreatedAt.Before(cutoff) {
			continue
		}
		if _, highRisk := enterpriseAdministrationHighRiskActions[entry.Action]; highRisk {
			count++
		}
	}
	return count, nil
}
