package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestDedicatedTenantMemoryConfigOwnsGenerationAndGenericUpdatesCannotOverwrite(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	tenant := &types.Tenant{Name: "workspace", Status: "active"}
	require.NoError(t, db.Create(tenant).Error)

	memoryRepo := NewTenantMemoryConfigRepository(db)
	wanted := &types.TenantMemoryConfig{Enabled: true, WriteMode: types.MemoryWriteAuto}
	first, err := memoryRepo.Update(ctx, tenant.ID, wanted)
	require.NoError(t, err)
	require.Equal(t, int64(1), first.Generation)
	require.True(t, first.Config.Enabled)
	require.Equal(t, types.DefaultMemoryMaxItems, first.Config.MaxItems)

	second, err := memoryRepo.Update(ctx, tenant.ID, wanted)
	require.NoError(t, err)
	require.Equal(t, int64(1), second.Generation, "an identical policy write is a no-op")

	staleWholeTenant := *tenant
	staleWholeTenant.Name = "renamed"
	staleWholeTenant.MemoryConfig = &types.TenantMemoryConfig{Enabled: false}
	staleWholeTenant.MemoryGeneration = 0
	require.NoError(t, NewTenantRepository(db).UpdateTenant(ctx, &staleWholeTenant))

	current, err := memoryRepo.Get(ctx, tenant.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), current.Generation)
	require.True(t, current.Config.Enabled)
	require.Equal(t, types.DefaultMemoryMaxItems, current.Config.MaxItems)
}

func TestPlatformRuntimeGenerationComposesWithoutMutatingTenantRows(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	tenantOne := &types.Tenant{Name: "one", Status: "active"}
	tenantTwo := &types.Tenant{Name: "two", Status: "active"}
	require.NoError(t, db.Create(tenantOne).Error)
	require.NoError(t, db.Create(tenantTwo).Error)

	platformRepo := NewPlatformMemoryRuntimeConfigRepository(db)
	runtime := types.DefaultMemoryRuntimeConfig()
	runtime.MaxItems = 333
	updated, err := platformRepo.Update(ctx, runtime, "platform-admin", time.Unix(100, 0), nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), updated.Generation)

	memoryRepo := NewTenantMemoryConfigRepository(db)
	first, err := memoryRepo.Get(ctx, tenantOne.ID)
	require.NoError(t, err)
	second, err := memoryRepo.Get(ctx, tenantTwo.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), first.Generation)
	require.Equal(t, int64(1), second.Generation)
	require.Equal(t, 333, first.Config.MaxItems)
	require.Equal(t, 333, second.Config.MaxItems)
	require.Zero(t, first.TenantGeneration)
	require.Zero(t, second.TenantGeneration)

	consent := &types.TenantMemoryConfig{Enabled: true, WriteMode: types.MemoryWriteAuto}
	first, err = memoryRepo.Update(ctx, tenantOne.ID, consent)
	require.NoError(t, err)
	require.Equal(t, int64(2), first.Generation)
	second, err = memoryRepo.Get(ctx, tenantTwo.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), second.Generation)

	trueValue := true
	equivalent := *runtime
	equivalent.VectorRecall = &trueValue
	equivalent.RetrievalConditioning = &trueValue
	updated, err = platformRepo.Update(ctx, &equivalent, "platform-admin", time.Unix(200, 0), nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), updated.Generation, "semantic no-op must not advance generation")

	newTenant := &types.Tenant{Name: "new", Status: "active"}
	require.NoError(t, db.Create(newTenant).Error)
	created, err := memoryRepo.Get(ctx, newTenant.ID)
	require.NoError(t, err)
	require.Equal(t, 333, created.Config.MaxItems)
	require.False(t, created.Config.Enabled)
	require.Equal(t, int64(1), created.Generation)
}
