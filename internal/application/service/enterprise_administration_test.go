package service

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type enterpriseTenantStub struct{ tenant *types.Tenant }

func (s enterpriseTenantStub) GetTenantByID(context.Context, uint64) (*types.Tenant, error) {
	return s.tenant, nil
}

type enterpriseMembersStub struct{ members []*types.TenantMember }

func (s enterpriseMembersStub) ListByTenant(context.Context, uint64) ([]*types.TenantMember, error) {
	return s.members, nil
}

type enterpriseInvitationsStub struct{ invitations []*types.TenantInvitation }

func (s enterpriseInvitationsStub) ListByTenant(context.Context, uint64, bool) ([]*types.TenantInvitation, error) {
	return s.invitations, nil
}

type enterpriseKBStub struct{ knowledgeBases []*types.KnowledgeBase }

func (s enterpriseKBStub) ListKnowledgeBasesByTenantID(context.Context, uint64) ([]*types.KnowledgeBase, error) {
	return s.knowledgeBases, nil
}

type enterpriseKnowledgeCountStub struct{ count int64 }

func (s enterpriseKnowledgeCountStub) CountKnowledgeByStatus(
	context.Context,
	uint64,
	string,
	[]string,
) (int64, error) {
	return s.count, nil
}

type enterpriseAuditStub struct{ entries []*types.AuditLog }

func (s enterpriseAuditStub) List(
	context.Context,
	uint64,
	*interfaces.AuditLogQuery,
) ([]*types.AuditLog, error) {
	return s.entries, nil
}

type enterprisePlatformStub struct {
	projection *types.EnterpriseAdministrationPlatformProjection
	facts      types.EnterpriseAdministrationFacts
}

func (s *enterprisePlatformStub) ResolveEnterpriseAdministrationQueue(
	_ context.Context,
	_ uint64,
	facts types.EnterpriseAdministrationFacts,
) (*types.EnterpriseAdministrationPlatformProjection, error) {
	s.facts = facts
	return s.projection, nil
}

func enterpriseAdministrationContext(role types.TenantRole) context.Context {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	return context.WithValue(ctx, types.TenantRoleContextKey, role)
}

func enterprisePlatformProjection(items ...types.EnterpriseAdministrationItem) *types.EnterpriseAdministrationPlatformProjection {
	quota := 20
	return &types.EnterpriseAdministrationPlatformProjection{
		ContractVersion: types.EnterpriseAdministrationQueueV1,
		AsOf:            "2026-08-30T00:00:00Z",
		Summary: types.EnterpriseAdministrationPlatformSummary{
			ServiceLevel: "retail_agent_enterprise",
			Status:       "active",
			MemberQuota:  &quota,
			Health:       "healthy",
		},
		Items: items,
	}
}

func TestEnterpriseAdministrationQueueFiltersAdminAndKnowledgeRoles(t *testing.T) {
	platform := &enterprisePlatformStub{projection: enterprisePlatformProjection(
		types.EnterpriseAdministrationItem{
			Code: "operating_analysis_access_gap", Priority: "high", Count: 2, Target: "members",
		},
	)}
	svc := &enterpriseAdministrationService{
		tenant: enterpriseTenantStub{tenant: &types.Tenant{ID: 7, StorageUsed: 100, StorageQuota: 1000}},
		members: enterpriseMembersStub{members: []*types.TenantMember{
			{Status: types.TenantMemberStatusActive, OperatingAnalysisAccess: true},
			{Status: types.TenantMemberStatusActive},
			{Status: types.TenantMemberStatusActive},
		}},
		invitations: enterpriseInvitationsStub{invitations: []*types.TenantInvitation{
			{InviteeUserID: "user-1"},
			{InviteeUserID: ""},
		}},
		knowledge:    enterpriseKBStub{knowledgeBases: []*types.KnowledgeBase{{ID: "kb-1"}}},
		knowledgeCnt: enterpriseKnowledgeCountStub{count: 1},
		audit: enterpriseAuditStub{entries: []*types.AuditLog{
			{Action: types.AuditActionOwnershipTransferred, CreatedAt: time.Now()},
		}},
		platform: platform,
	}

	adminQueue, err := svc.Resolve(enterpriseAdministrationContext(types.TenantRoleAdmin))
	require.NoError(t, err)
	require.Equal(t, types.EnterpriseAdministrationFacts{
		Role:                                "admin",
		ActiveMemberCount:                   3,
		OperatingAnalysisMissingAccessCount: 2,
	}, platform.facts)
	require.Equal(t, []string{
		"operating_analysis_access_gap",
		"knowledge_processing_failed",
		"pending_invitations",
		"recent_high_risk_operations",
	}, []string{
		adminQueue.Items[0].Code,
		adminQueue.Items[1].Code,
		adminQueue.Items[2].Code,
		adminQueue.Items[3].Code,
	})
	require.Equal(t, int64(100), adminQueue.Summary.StorageUsageByte)

	platform.projection = enterprisePlatformProjection()
	_, err = svc.Resolve(enterpriseAdministrationContext(types.TenantRoleContributor))
	require.Error(t, err)
}

func TestEnterpriseAdministrationQueueRejectsEmployee(t *testing.T) {
	svc := &enterpriseAdministrationService{}
	_, err := svc.Resolve(enterpriseAdministrationContext(types.TenantRoleViewer))
	require.Error(t, err)
}
