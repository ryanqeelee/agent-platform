package repository

import (
	"context"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureDefaultWebSearchProviderStatePolicy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:web-search-default?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.WebSearchProviderEntity{}))
	repo := NewWebSearchProviderRepository(db)
	ctx := context.Background()

	createTenant := func(t *testing.T, id uint64) {
		t.Helper()
		require.NoError(t, db.Create(&types.Tenant{ID: id, Name: "tenant"}).Error)
	}

	t.Run("zero history creates canonical keyless default", func(t *testing.T) {
		createTenant(t, 1)
		provider, err := repo.EnsureDefault(ctx, 1)
		require.NoError(t, err)
		require.NotNil(t, provider)
		require.Equal(t, uint64(1), provider.TenantID)
		require.Equal(t, "Keenable", provider.Name)
		require.Equal(t, types.WebSearchProviderTypeKeenable, provider.Provider)
		require.Empty(t, provider.Parameters.APIKey)
		require.Empty(t, provider.Parameters.BaseURL)
		require.True(t, provider.IsDefault)

		replay, err := repo.EnsureDefault(ctx, 1)
		require.NoError(t, err)
		require.NotNil(t, replay)
		require.Equal(t, provider.ID, replay.ID)
	})

	t.Run("existing default is preserved", func(t *testing.T) {
		createTenant(t, 2)
		existing := &types.WebSearchProviderEntity{
			TenantID: 2, Name: "Admin Bing", Provider: types.WebSearchProviderTypeBing,
			Parameters: types.WebSearchProviderParameters{APIKey: "configured"}, IsDefault: true,
		}
		require.NoError(t, repo.Create(ctx, existing))

		provider, err := repo.EnsureDefault(ctx, 2)
		require.NoError(t, err)
		require.NotNil(t, provider)
		require.Equal(t, existing.ID, provider.ID)
		require.Equal(t, types.WebSearchProviderTypeBing, provider.Provider)
	})

	t.Run("existing provider without default remains unavailable", func(t *testing.T) {
		createTenant(t, 3)
		existing := &types.WebSearchProviderEntity{
			TenantID: 3, Name: "Disabled default", Provider: types.WebSearchProviderTypeKeenable,
		}
		require.NoError(t, repo.Create(ctx, existing))

		provider, err := repo.EnsureDefault(ctx, 3)
		require.NoError(t, err)
		require.Nil(t, provider)
		providers, err := repo.List(ctx, 3)
		require.NoError(t, err)
		require.Len(t, providers, 1)
		require.Equal(t, existing.ID, providers[0].ID)
	})

	t.Run("soft-deleted history prevents reprovisioning", func(t *testing.T) {
		createTenant(t, 4)
		removed := &types.WebSearchProviderEntity{
			TenantID: 4, Name: "Removed", Provider: types.WebSearchProviderTypeKeenable, IsDefault: true,
		}
		require.NoError(t, repo.Create(ctx, removed))
		require.NoError(t, repo.Delete(ctx, 4, removed.ID))

		provider, err := repo.EnsureDefault(ctx, 4)
		require.NoError(t, err)
		require.Nil(t, provider)
		var count int64
		require.NoError(t, db.Unscoped().Model(&types.WebSearchProviderEntity{}).Where("tenant_id = ?", 4).Count(&count).Error)
		require.Equal(t, int64(1), count)
	})

	t.Run("missing tenant is rejected", func(t *testing.T) {
		provider, err := repo.EnsureDefault(ctx, 999)
		require.Error(t, err)
		require.Nil(t, provider)
	})
}

func TestEnsureDefaultWebSearchProviderConcurrentPostgres(t *testing.T) {
	db := newIsolatedPostgresTestDatabase(t, "weknora_web_search_default_")
	require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.WebSearchProviderEntity{}))
	require.NoError(t, db.Create(&types.Tenant{ID: 1, Name: "tenant"}).Error)
	repo := NewWebSearchProviderRepository(db)

	const calls = 8
	providers := make([]*types.WebSearchProviderEntity, calls)
	errs := make([]error, calls)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := range providers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			providers[index], errs[index] = repo.EnsureDefault(context.Background(), 1)
		}(index)
	}
	close(start)
	wg.Wait()

	for index := range providers {
		require.NoError(t, errs[index])
		require.NotNil(t, providers[index])
		require.Equal(t, providers[0].ID, providers[index].ID)
	}
	var count int64
	require.NoError(t, db.Model(&types.WebSearchProviderEntity{}).Where("tenant_id = ?", 1).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestDefaultKeenableMigrationPreservesProviderHistory(t *testing.T) {
	db := newIsolatedPostgresTestDatabase(t, "weknora_web_search_migration_")
	for _, statement := range []string{
		`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`,
		`CREATE TABLE tenants (id bigint PRIMARY KEY, deleted_at timestamptz)`,
		`CREATE TABLE web_search_providers (
			id varchar(36) PRIMARY KEY, tenant_id bigint NOT NULL, name varchar(255) NOT NULL,
			provider varchar(50) NOT NULL, description text, parameters jsonb,
			is_default boolean DEFAULT false, created_at timestamptz, updated_at timestamptz,
			deleted_at timestamptz
		)`,
		`INSERT INTO tenants(id, deleted_at) VALUES (1, NULL), (2, NULL), (3, NULL), (4, CURRENT_TIMESTAMP)`,
		`INSERT INTO web_search_providers(id, tenant_id, name, provider, parameters, is_default, deleted_at)
		 VALUES ('admin', 2, 'Admin Bing', 'bing', '{}', false, NULL),
		        ('removed', 3, 'Removed', 'keenable', '{}', true, CURRENT_TIMESTAMP)`,
	} {
		require.NoError(t, db.Exec(statement).Error)
	}

	migration := migrationSQL(t, "migrations/versioned/000098_default_keenable_web_search.up.sql")
	require.NoError(t, db.Exec(migration).Error)
	require.NoError(t, db.Exec(migration).Error)

	var total, tenantOneDefaults, tenantTwoRows, tenantThreeRows, tenantFourRows int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM web_search_providers`).Scan(&total).Error)
	require.NoError(t, db.Raw(`SELECT count(*) FROM web_search_providers
		WHERE tenant_id = 1 AND provider = 'keenable' AND is_default = TRUE AND parameters = '{}'::jsonb`).Scan(&tenantOneDefaults).Error)
	require.NoError(t, db.Raw(`SELECT count(*) FROM web_search_providers
		WHERE tenant_id = 2 AND id = 'admin' AND provider = 'bing' AND is_default = FALSE`).Scan(&tenantTwoRows).Error)
	require.NoError(t, db.Raw(`SELECT count(*) FROM web_search_providers
		WHERE tenant_id = 3 AND id = 'removed' AND deleted_at IS NOT NULL`).Scan(&tenantThreeRows).Error)
	require.NoError(t, db.Raw(`SELECT count(*) FROM web_search_providers WHERE tenant_id = 4`).Scan(&tenantFourRows).Error)
	require.Equal(t, int64(3), total)
	require.Equal(t, int64(1), tenantOneDefaults)
	require.Equal(t, int64(1), tenantTwoRows)
	require.Equal(t, int64(1), tenantThreeRows)
	require.Zero(t, tenantFourRows)
}
