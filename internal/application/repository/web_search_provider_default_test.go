package repository

import (
	"context"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newGlobalWebSearchRepository(t *testing.T) (*gorm.DB, interfaces.WebSearchProviderRepository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.WebSearchProviderEntity{}))
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_web_search_providers_single_default_test
		ON web_search_providers(is_default) WHERE is_default = 1 AND deleted_at IS NULL`).Error)
	return db, NewWebSearchProviderRepository(db)
}

func TestGlobalWebSearchDefaultRequiresExplicitExactID(t *testing.T) {
	_, repo := newGlobalWebSearchRepository(t)
	ctx := context.Background()
	first := &types.WebSearchProviderEntity{ID: "first", Name: "first", Provider: types.WebSearchProviderTypeKeenable}
	second := &types.WebSearchProviderEntity{ID: "second", Name: "second", Provider: types.WebSearchProviderTypeKeenable}
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Create(ctx, second))

	provider, err := repo.GetDefault(ctx)
	require.NoError(t, err)
	require.Nil(t, provider, "an unset default must not choose an arbitrary row")
	require.ErrorContains(t, repo.SetDefault(ctx, "missing"), "web search provider not found")

	require.NoError(t, repo.SetDefault(ctx, second.ID))
	provider, err = repo.GetDefault(ctx)
	require.NoError(t, err)
	require.Equal(t, second.ID, provider.ID)

	require.NoError(t, repo.SetDefault(ctx, first.ID))
	rows, err := repo.List(ctx)
	require.NoError(t, err)
	require.True(t, rows[0].IsDefault)
	require.False(t, rows[1].IsDefault)
	require.ErrorContains(t, repo.Delete(ctx, first.ID), "default web search provider cannot be deleted")
	require.NoError(t, repo.Delete(ctx, second.ID))
}

func TestGlobalWebSearchDefaultConcurrentPostgres(t *testing.T) {
	db := newIsolatedPostgresTestDatabase(t, "weknora_web_search_default_")
	require.NoError(t, db.Exec(`CREATE TABLE web_search_providers (
		id varchar(36) PRIMARY KEY, name varchar(255) NOT NULL, provider varchar(50) NOT NULL,
		description text, parameters jsonb, is_default boolean NOT NULL DEFAULT false,
		created_at timestamptz, updated_at timestamptz, deleted_at timestamptz
	)`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_web_search_providers_single_default
		ON web_search_providers(is_default) WHERE is_default = TRUE AND deleted_at IS NULL`).Error)
	require.NoError(t, db.Exec(`INSERT INTO web_search_providers(id, name, provider, parameters)
		VALUES ('first', 'first', 'keenable', '{}'::jsonb), ('second', 'second', 'keenable', '{}'::jsonb)`).Error)
	repo := NewWebSearchProviderRepository(db)

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{"first", "second"} {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			<-start
			errs[i] = repo.SetDefault(context.Background(), id)
		}(i, id)
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM web_search_providers WHERE is_default = TRUE`).Scan(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestGlobalWebSearchDefaultPostgresSwitchAndDeleteGuard(t *testing.T) {
	db := newIsolatedPostgresTestDatabase(t, "weknora_web_search_switch_")
	require.NoError(t, db.Exec(`CREATE TABLE web_search_providers (
		id varchar(36) PRIMARY KEY, name varchar(255) NOT NULL, provider varchar(50) NOT NULL,
		description text, parameters jsonb, is_default boolean NOT NULL DEFAULT false,
		created_at timestamptz, updated_at timestamptz, deleted_at timestamptz
	)`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_web_search_providers_single_default
		ON web_search_providers(is_default) WHERE is_default = TRUE AND deleted_at IS NULL`).Error)
	require.NoError(t, db.Exec(`INSERT INTO web_search_providers(id, name, provider, parameters)
		VALUES ('first', 'first', 'keenable', '{}'::jsonb), ('second', 'second', 'keenable', '{}'::jsonb)`).Error)
	repo := NewWebSearchProviderRepository(db)

	require.NoError(t, repo.SetDefault(context.Background(), "first"))
	require.ErrorContains(t, repo.Delete(context.Background(), "first"), "default web search provider cannot be deleted")
	require.NoError(t, repo.SetDefault(context.Background(), "second"))
	require.NoError(t, repo.Delete(context.Background(), "first"))

	provider, err := repo.GetDefault(context.Background())
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.Equal(t, "second", provider.ID)
	deleted, err := repo.GetByID(context.Background(), "first")
	require.NoError(t, err)
	require.Nil(t, deleted)
}

func TestGlobalWebSearchMigrationPreservesIDsAndLeavesDefaultUnset(t *testing.T) {
	db := newIsolatedPostgresTestDatabase(t, "weknora_web_search_global_")
	require.NoError(t, db.Exec(`CREATE TABLE tenants (
		id bigint PRIMARY KEY, web_search_config jsonb
	)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE web_search_providers (
		id varchar(36) PRIMARY KEY, tenant_id bigint NOT NULL, name varchar(255) NOT NULL,
		provider varchar(50) NOT NULL, description text, parameters jsonb,
		is_default boolean DEFAULT false, created_at timestamptz, updated_at timestamptz,
		deleted_at timestamptz
	)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO web_search_providers(id, tenant_id, name, provider, parameters, is_default)
		VALUES ('p1', 10001, 'first', 'keenable', '{}'::jsonb, true),
		       ('p2', 10004, 'second', 'keenable', '{}'::jsonb, true)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO tenants(id, web_search_config)
		VALUES (10001, '{"api_key":"retired"}'::jsonb)`).Error)

	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000110_global_web_search_providers.up.sql")).Error)
	var ids []string
	require.NoError(t, db.Raw(`SELECT id FROM web_search_providers ORDER BY id`).Scan(&ids).Error)
	require.Equal(t, []string{"p1", "p2"}, ids)
	var defaultCount int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM web_search_providers WHERE is_default = TRUE`).Scan(&defaultCount).Error)
	require.Zero(t, defaultCount)
	var tenantColumns int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'web_search_providers' AND column_name = 'tenant_id'`).Scan(&tenantColumns).Error)
	require.Zero(t, tenantColumns)
	require.NoError(t, db.Raw(`SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'tenants' AND column_name = 'web_search_config'`).Scan(&tenantColumns).Error)
	require.Zero(t, tenantColumns)
}
