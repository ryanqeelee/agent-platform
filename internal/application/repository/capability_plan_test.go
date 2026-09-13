package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func capabilityPlanTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared&_foreign_keys=1"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.Tenant{}, &types.User{}, &types.AuditLog{},
		&types.AICapabilityPlanVersion{}, &types.AICapabilityPlanDefault{},
		&types.TenantAICapabilityPlanAssignment{},
		&types.AssistantScenarioCapabilityDefault{},
		&types.TenantAssistantScenarioCapabilityOverride{},
	))
	return db
}

func capabilityPlanTestAuthority(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Omit("tenant_id").Create(&types.User{
		ID: "system-admin", Username: "system-admin", Email: "system-admin@example.invalid",
		IsActive: true, IsSystemAdmin: true,
	}).Error)
}

func capabilityPlanTestVersion() *types.AICapabilityPlanVersion {
	return &types.AICapabilityPlanVersion{
		VersionID: "plan-v1", ContractVersion: types.AICapabilityPlanContractVersion,
		ServiceLevel: "standard", EmployeeAssistantRequestRuntimeRef: "assistant-ref",
		OperatingAnalysisRequestRuntimeRef: "analysis-ref", EmbeddingRef: "embedding-ref",
		RerankingRef: "reranking-ref", ParsingRef: "parsing-ref",
		CreatedBy: "system-admin", CreatedAt: time.Now().UTC(),
	}
}

func TestCapabilityPlanRepositoryResolvesAssignmentBeforeDefault(t *testing.T) {
	db := capabilityPlanTestDB(t)
	capabilityPlanTestAuthority(t, db)
	repo := NewCapabilityPlanRepository(db)
	ctx := context.Background()
	tenant := &types.Tenant{Name: "Acme", Status: types.TenantStatusActive}
	require.NoError(t, db.Create(tenant).Error)
	require.NoError(t, repo.CreatePlan(ctx, "system-admin", capabilityPlanTestVersion()))
	second := capabilityPlanTestVersion()
	second.VersionID = "plan-v2"
	require.NoError(t, repo.CreatePlan(ctx, "system-admin", second))
	require.NoError(t, repo.SetDefaultPlan(ctx, "system-admin", "plan-v1", time.Now()))
	require.NoError(t, repo.AssignTenantPlan(ctx, "system-admin", tenant.ID, "plan-v2", time.Now()))

	resolved, err := repo.ResolvePlan(ctx, tenant.ID)
	require.NoError(t, err)
	require.Equal(t, "plan-v2", resolved.Plan.VersionID)
	require.Equal(t, "tenant_assignment", resolved.Source)

	require.NoError(t, repo.ClearTenantPlan(ctx, "system-admin", tenant.ID, time.Now()))
	resolved, err = repo.ResolvePlan(ctx, tenant.ID)
	require.NoError(t, err)
	require.Equal(t, "plan-v1", resolved.Plan.VersionID)
	require.Equal(t, "platform_default", resolved.Source)
}

func TestCapabilityPlanRepositoryPreservesExplicitFalseScenarioOverride(t *testing.T) {
	db := capabilityPlanTestDB(t)
	capabilityPlanTestAuthority(t, db)
	repo := NewCapabilityPlanRepository(db)
	ctx := context.Background()
	tenant := &types.Tenant{Name: "Acme", Status: types.TenantStatusActive}
	require.NoError(t, db.Create(tenant).Error)
	require.NoError(t, repo.SetDefaultScenario(ctx, "system-admin", types.AssistantScenarioCapabilities{
		ExternalSearch: true, MCP: true, Tools: true,
	}, time.Now()))
	require.NoError(t, repo.SetTenantScenario(ctx, "system-admin", tenant.ID, types.AssistantScenarioCapabilities{}, time.Now()))

	resolved, err := repo.ResolveScenario(ctx, tenant.ID)
	require.NoError(t, err)
	require.Equal(t, "tenant_assignment", resolved.Source)
	require.False(t, resolved.Capabilities.ExternalSearch)
	require.False(t, resolved.Capabilities.MCP)
	require.False(t, resolved.Capabilities.Tools)
}

func TestCapabilityPlanRepositoryRejectsRevokedSystemAdministratorWrite(t *testing.T) {
	db := capabilityPlanTestDB(t)
	capabilityPlanTestAuthority(t, db)
	repo := NewCapabilityPlanRepository(db)
	require.NoError(t, db.Model(&types.User{}).Where("id = ?", "system-admin").Update("is_system_admin", false).Error)

	err := repo.CreatePlan(context.Background(), "system-admin", capabilityPlanTestVersion())
	require.ErrorIs(t, err, ErrMemberActionForbidden)
	var count int64
	require.NoError(t, db.Model(&types.AICapabilityPlanVersion{}).Count(&count).Error)
	require.Zero(t, count)
}
