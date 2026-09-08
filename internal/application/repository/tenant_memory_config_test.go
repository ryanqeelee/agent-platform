package repository

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestDedicatedTenantMemoryConfigOwnsGenerationAndGenericUpdatesCannotOverwrite(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	tenant := &types.Tenant{Name: "workspace", Status: "active"}
	require.NoError(t, db.Create(tenant).Error)

	memoryRepo := NewTenantMemoryConfigRepository(db)
	wanted := &types.MemoryConfig{Enabled: true, WriteMode: types.MemoryWriteAuto, MaxItems: 321}
	first, err := memoryRepo.Update(ctx, tenant.ID, wanted)
	require.NoError(t, err)
	require.Equal(t, int64(1), first.Generation)
	require.True(t, first.Config.Enabled)
	require.Equal(t, 321, first.Config.MaxItems)

	second, err := memoryRepo.Update(ctx, tenant.ID, wanted)
	require.NoError(t, err)
	require.Equal(t, int64(1), second.Generation, "an identical policy write is a no-op")

	staleWholeTenant := *tenant
	staleWholeTenant.Name = "renamed"
	staleWholeTenant.MemoryConfig = &types.MemoryConfig{Enabled: false}
	staleWholeTenant.MemoryGeneration = 0
	require.NoError(t, NewTenantRepository(db).UpdateTenant(ctx, &staleWholeTenant))

	current, err := memoryRepo.Get(ctx, tenant.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), current.Generation)
	require.True(t, current.Config.Enabled)
	require.Equal(t, 321, current.Config.MaxItems)
}
