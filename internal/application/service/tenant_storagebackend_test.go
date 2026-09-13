package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateTenantDoesNotCreateStorageBackend(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.StorageBackend{}, &types.WebSearchProviderEntity{}))
	tenantRepo := repository.NewTenantRepository(db)
	tenantSvc := service.NewTenantService(tenantRepo)

	tenant, err := tenantSvc.CreateTenant(context.Background(), &types.Tenant{Name: "workspace"})
	require.NoError(t, err)
	require.NotZero(t, tenant.ID)
	var backendCount int64
	require.NoError(t, db.Model(&types.StorageBackend{}).Count(&backendCount).Error)
	require.Zero(t, backendCount)
}

func TestEnterpriseActivationDoesNotCreatePlatformStorageBackend(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.Tenant{}, &types.User{}, &types.TenantMember{}, &types.StorageBackend{},
		&types.WebSearchProviderEntity{}, &types.AICapabilityPlanVersion{},
		&types.TenantAICapabilityPlanAssignment{},
	))
	tenantRepo := repository.NewTenantRepository(db)
	tenantSvc := service.NewTenantService(tenantRepo)
	require.NoError(t, db.Create(&types.User{
		ID: "activation-owner", Username: "activation-owner", Email: "activation-owner@example.invalid", IsActive: true,
	}).Error)
	require.NoError(t, db.Omit("TenantID").Create(&types.User{
		ID: "activation-system-admin", Username: "activation-system-admin",
		Email: "activation-system-admin@example.invalid", IsActive: true, IsSystemAdmin: true,
	}).Error)
	require.NoError(t, db.Create(&types.AICapabilityPlanVersion{
		VersionID: "activation-plan-v1", ContractVersion: types.AICapabilityPlanContractVersion,
		ServiceLevel: "test", CreatedBy: "activation-system-admin",
	}).Error)
	command := interfaces.EnterpriseActivationCommand{
		ActivationID: "activation-storage", ActorUserID: "activation-system-admin",
		IdempotencyKeySHA256: strings.Repeat("b", 64), RequestSHA256: strings.Repeat("a", 64),
		AICapabilityPlanVersionID: "activation-plan-v1",
		TenantName:                "Acme", TenantDescription: "Acme workspace",
		FirstOwnerUserID: "activation-owner", DesiredState: types.EnterpriseActivationStatePrepared,
	}

	activationSvc := tenantSvc.(interfaces.EnterpriseActivationService)
	prepared, err := activationSvc.ApplyEnterpriseActivation(context.Background(), command)
	require.NoError(t, err)
	require.Equal(t, types.EnterpriseActivationStatePrepared, prepared.State)

	command.DesiredState = types.EnterpriseActivationStateActive
	result, err := activationSvc.ApplyEnterpriseActivation(context.Background(), command)
	require.NoError(t, err)
	require.Equal(t, types.EnterpriseActivationStateActive, result.State)

	replayed, err := activationSvc.ApplyEnterpriseActivation(context.Background(), command)
	require.NoError(t, err)
	require.Equal(t, result.TenantID, replayed.TenantID)
	require.Equal(t, result.OwnerMembershipID, replayed.OwnerMembershipID)
	require.Equal(t, types.EnterpriseActivationStateActive, replayed.State)

	var member types.TenantMember
	require.NoError(t, db.First(&member, result.OwnerMembershipID).Error)
	require.Equal(t, types.TenantRoleAdmin, member.Role)
	require.Equal(t, types.TenantMemberStatusActive, member.Status)

	var tenantCount, memberCount, backendCount int64
	require.NoError(t, db.Model(&types.Tenant{}).Count(&tenantCount).Error)
	require.NoError(t, db.Model(&types.TenantMember{}).Count(&memberCount).Error)
	require.NoError(t, db.Model(&types.StorageBackend{}).Count(&backendCount).Error)
	require.Equal(t, int64(1), tenantCount)
	require.Equal(t, int64(1), memberCount)
	require.Zero(t, backendCount)
}
