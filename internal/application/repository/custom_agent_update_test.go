package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomAgentRepositoryUpdateAgentScopesCompositeKeyAndPersistsZeroValues(t *testing.T) {
	ctx := context.Background()
	db := setupModelUsageTestDB(t)
	repo := NewCustomAgentRepository(db)
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)

	global := &types.CustomAgent{
		ID:          "shared-agent",
		TenantID:    0,
		Name:        "global-original",
		Description: "global description",
		Avatar:      "global-avatar",
		IsBuiltin:   true,
		CreatedBy:   "platform-admin",
		Config:      types.CustomAgentConfig{ModelID: "global-model"},
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	tenant := &types.CustomAgent{
		ID:          global.ID,
		TenantID:    42,
		Name:        "tenant-original",
		Description: "tenant description",
		Avatar:      "tenant-avatar",
		CreatedBy:   "tenant-admin",
		Config:      types.CustomAgentConfig{ModelID: "tenant-model"},
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	require.NoError(t, repo.CreateAgent(ctx, global))
	require.NoError(t, repo.CreateAgent(ctx, tenant))

	global.Name = "global-first-update"
	global.Description = "updated"
	global.Config = types.CustomAgentConfig{ModelID: "updated-model"}
	global.UpdatedAt = createdAt.Add(time.Hour)
	require.NoError(t, repo.UpdateAgent(ctx, global))

	global.Name = "global-second-update"
	global.Description = ""
	global.Avatar = ""
	global.IsBuiltin = false
	global.CreatedBy = ""
	global.Config = types.CustomAgentConfig{}
	global.UpdatedAt = createdAt.Add(2 * time.Hour)
	require.NoError(t, repo.UpdateAgent(ctx, global))

	gotGlobal, err := repo.GetAgentByID(ctx, global.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, "global-second-update", gotGlobal.Name)
	assert.Empty(t, gotGlobal.Description)
	assert.Empty(t, gotGlobal.Avatar)
	assert.False(t, gotGlobal.IsBuiltin)
	assert.Empty(t, gotGlobal.CreatedBy)
	assert.Equal(t, types.CustomAgentConfig{}, gotGlobal.Config)
	assert.Equal(t, createdAt, gotGlobal.CreatedAt)

	gotTenant, err := repo.GetAgentByID(ctx, tenant.ID, tenant.TenantID)
	require.NoError(t, err)
	assert.Equal(t, "tenant-original", gotTenant.Name)
	assert.Equal(t, "tenant description", gotTenant.Description)
	assert.Equal(t, "tenant-avatar", gotTenant.Avatar)
	assert.Equal(t, "tenant-admin", gotTenant.CreatedBy)
	assert.Equal(t, "tenant-model", gotTenant.Config.ModelID)

	gotTenant.Name = "tenant-updated"
	gotTenant.Description = ""
	gotTenant.Config = types.CustomAgentConfig{}
	gotTenant.UpdatedAt = createdAt.Add(3 * time.Hour)
	require.NoError(t, repo.UpdateAgent(ctx, gotTenant))

	updatedTenant, err := repo.GetAgentByID(ctx, tenant.ID, tenant.TenantID)
	require.NoError(t, err)
	assert.Equal(t, "tenant-updated", updatedTenant.Name)
	assert.Empty(t, updatedTenant.Description)
	assert.Equal(t, types.CustomAgentConfig{}, updatedTenant.Config)
}

func TestCustomAgentRepositoryUpdateAgentDoesNotCreateMissingRecord(t *testing.T) {
	ctx := context.Background()
	db := setupModelUsageTestDB(t)
	repo := NewCustomAgentRepository(db)

	err := repo.UpdateAgent(ctx, &types.CustomAgent{
		ID:       "missing-agent",
		TenantID: 0,
		Name:     "must-not-be-created",
	})
	require.ErrorIs(t, err, ErrCustomAgentNotFound)

	var count int64
	require.NoError(t, db.Model(&types.CustomAgent{}).
		Where("id = ? AND tenant_id = ?", "missing-agent", 0).
		Count(&count).Error)
	assert.Zero(t, count)
}
