package service_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateTenantCreatesConcreteDefaultStorageBackend(t *testing.T) {
	t.Setenv("STORAGE_TYPE", "local")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.StorageBackend{}, &types.WebSearchProviderEntity{}))
	tenantRepo := repository.NewTenantRepository(db)
	storageRepo := repository.NewStorageBackendRepository(db)
	webSearchRepo := repository.NewWebSearchProviderRepository(db)
	tenantSvc := service.NewTenantService(tenantRepo, storageRepo, webSearchRepo)

	tenant, err := tenantSvc.CreateTenant(context.Background(), &types.Tenant{Name: "workspace"})
	require.NoError(t, err)
	require.NotNil(t, tenant.DefaultStorageBackendID)

	backend, err := storageRepo.GetByID(context.Background(), tenant.ID, *tenant.DefaultStorageBackendID)
	require.NoError(t, err)
	require.NotNil(t, backend)
	assert.Equal(t, "local", backend.Provider)
	assert.Equal(t, types.StorageBackendSourceEnv, backend.Source)
	assert.True(t, backend.LegacyAlias)
	searchProvider, err := webSearchRepo.GetDefault(context.Background(), tenant.ID)
	require.NoError(t, err)
	require.NotNil(t, searchProvider)
	assert.Equal(t, types.WebSearchProviderTypeKeenable, searchProvider.Provider)
	assert.Empty(t, searchProvider.Parameters.APIKey)
}

func TestEnterpriseActivationResumesDefaultBackendBeforeOpening(t *testing.T) {
	t.Setenv("STORAGE_TYPE", "unsupported-for-test")
	db, err := gorm.Open(sqlite.Open("file:activation-storage?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.Tenant{}, &types.User{}, &types.TenantMember{}, &types.StorageBackend{},
		&types.WebSearchProviderEntity{}, &types.AICapabilityPlanVersion{},
		&types.TenantAICapabilityPlanAssignment{},
	))
	tenantRepo := repository.NewTenantRepository(db)
	storageRepo := repository.NewStorageBackendRepository(db)
	webSearchRepo := repository.NewWebSearchProviderRepository(db)
	tenantSvc := service.NewTenantService(tenantRepo, storageRepo, webSearchRepo)
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

	// The durable receipt remains prepared when backend materialization fails.
	activationSvc := tenantSvc.(interfaces.EnterpriseActivationService)
	_, err = activationSvc.ApplyEnterpriseActivation(context.Background(), command)
	require.Error(t, err)
	var prepared types.Tenant
	require.NoError(t, db.Where("ringxun_activation_id = ?", command.ActivationID).Take(&prepared).Error)
	require.Equal(t, types.TenantStatusProvisioning, prepared.Status)
	require.Nil(t, prepared.DefaultStorageBackendID)

	require.NoError(t, os.Setenv("STORAGE_TYPE", "local"))
	command.DesiredState = types.EnterpriseActivationStateActive
	result, err := activationSvc.ApplyEnterpriseActivation(context.Background(), command)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, types.EnterpriseActivationStateActive, result.State)
	require.NoError(t, db.First(&prepared, result.TenantID).Error)
	require.NotNil(t, prepared.DefaultStorageBackendID)
	backend, err := storageRepo.GetByID(context.Background(), prepared.ID, *prepared.DefaultStorageBackendID)
	require.NoError(t, err)
	require.NotNil(t, backend)
	var member types.TenantMember
	require.NoError(t, db.First(&member, result.OwnerMembershipID).Error)
	require.Equal(t, types.TenantMemberStatusActive, member.Status)
	var tenantCount, memberCount, backendCount int64
	require.NoError(t, db.Model(&types.Tenant{}).Count(&tenantCount).Error)
	require.NoError(t, db.Model(&types.TenantMember{}).Count(&memberCount).Error)
	require.NoError(t, db.Model(&types.StorageBackend{}).Count(&backendCount).Error)
	searchProvider, err := webSearchRepo.GetDefault(context.Background(), result.TenantID)
	require.NoError(t, err)
	require.NotNil(t, searchProvider)
	require.Equal(t, types.WebSearchProviderTypeKeenable, searchProvider.Provider)
	require.Equal(t, int64(1), tenantCount)
	require.Equal(t, int64(1), memberCount)
	require.Equal(t, int64(1), backendCount)
}
