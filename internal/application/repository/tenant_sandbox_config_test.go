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

func newSandboxConfigTestRepo(t *testing.T) (TenantSandboxConfigRepository, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.TenantSandboxConfigEntity{}))
	// AutoMigrate cannot express the partial unique index, so add it here to
	// match the production migration.
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_platform_sandbox_configs_active_default
		 ON platform_sandbox_configs (is_default) WHERE is_default = 1 AND deleted_at IS NULL`).Error)
	return NewTenantSandboxConfigRepository(db), db
}

func TestSandboxConfigRepoIsPlatformGlobalAndPreservesDuplicateNames(t *testing.T) {
	repo, _ := newSandboxConfigTestRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &types.TenantSandboxConfigEntity{
		ID:          "cfg-a",
		Name:        "prod",
		SandboxType: "e2b",
		Config:      &types.TenantSandboxConfig{SandboxType: "e2b"},
	}))
	require.NoError(t, repo.Create(ctx, &types.TenantSandboxConfigEntity{
		ID:          "cfg-b",
		Name:        "prod",
		SandboxType: "e2b",
		Config:      &types.TenantSandboxConfig{SandboxType: "e2b"},
	}))

	got, err := repo.GetByID(ctx, "cfg-b")
	require.NoError(t, err)
	require.Equal(t, "cfg-b", got.ID)

	list, err := repo.ListAll(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	require.Equal(t, "cfg-a", list[0].ID)
}

func TestSandboxConfigRepoSwitchesExplicitDefaultAndGuardsDeletion(t *testing.T) {
	repo, _ := newSandboxConfigTestRepo(t)
	ctx := context.Background()

	base := func(id string) *types.TenantSandboxConfigEntity {
		return &types.TenantSandboxConfigEntity{
			ID:          id,
			Name:        "prod",
			SandboxType: "e2b",
			Config:      &types.TenantSandboxConfig{SandboxType: "e2b"},
		}
	}
	require.NoError(t, repo.Create(ctx, base("cfg-1")))
	require.NoError(t, repo.Create(ctx, base("cfg-2")))
	require.NoError(t, repo.SetDefault(ctx, "cfg-1"))

	got, err := repo.GetDefault(ctx)
	require.NoError(t, err)
	require.Equal(t, "cfg-1", got.ID)
	require.ErrorIs(t, repo.SoftDelete(ctx, "cfg-1"), ErrDeleteDefaultSandboxConfig)

	require.NoError(t, repo.SetDefault(ctx, "cfg-2"))
	got, err = repo.GetDefault(ctx)
	require.NoError(t, err)
	require.Equal(t, "cfg-2", got.ID)
	require.NoError(t, repo.SoftDelete(ctx, "cfg-1"))
}

func TestSandboxConfigRepoSoftDeleteHidesRow(t *testing.T) {
	repo, _ := newSandboxConfigTestRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &types.TenantSandboxConfigEntity{
		ID:          "cfg-a",
		Name:        "prod",
		SandboxType: "e2b",
		Config:      &types.TenantSandboxConfig{SandboxType: "e2b"},
	}))
	require.NoError(t, repo.SoftDelete(ctx, "cfg-a"))

	got, err := repo.GetByID(ctx, "cfg-a")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestSandboxConfigRepoCordonRoundTrip(t *testing.T) {
	repo, _ := newSandboxConfigTestRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Create(ctx, &types.TenantSandboxConfigEntity{
		ID:          "cfg-a",
		Name:        "prod",
		SandboxType: "e2b",
		Config:      &types.TenantSandboxConfig{SandboxType: "e2b"},
	}))

	at := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, repo.SetCordon(ctx, "cfg-a", at))

	got, err := repo.GetByID(ctx, "cfg-a")
	require.NoError(t, err)
	require.NotNil(t, got.CordonedAt)
	require.True(t, got.IsCordoned(at.Add(time.Second), types.SandboxCordonLease))

	require.NoError(t, repo.ClearCordon(ctx, "cfg-a"))
	got, err = repo.GetByID(ctx, "cfg-a")
	require.NoError(t, err)
	require.Nil(t, got.CordonedAt)
}
