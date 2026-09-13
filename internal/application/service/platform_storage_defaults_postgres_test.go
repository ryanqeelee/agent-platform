package service

import (
	"context"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func platformStorageTestPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("WEKNORA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("WEKNORA_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	raw, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	schema := "platform_defaults_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { require.NoError(t, admin.Exec("DROP SCHEMA "+schema+" CASCADE").Error) })
	scoped := dsn + " search_path=" + schema
	if strings.Contains(dsn, "://") {
		parsed, err := url.Parse(dsn)
		require.NoError(t, err)
		q := parsed.Query()
		q.Set("search_path", schema)
		parsed.RawQuery = q.Encode()
		scoped = parsed.String()
	}
	db, err := gorm.Open(postgres.Open(scoped), &gorm.Config{})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	require.NoError(t, db.AutoMigrate(&types.StorageBackend{}, &types.VectorStore{}))
	for _, sql := range []string{
		"CREATE UNIQUE INDEX storage_one_default ON storage_backends(is_default) WHERE is_default AND deleted_at IS NULL",
		"CREATE UNIQUE INDEX vector_one_default ON vector_stores(is_default) WHERE is_default AND deleted_at IS NULL",
		"CREATE TABLE knowledge_bases(id text PRIMARY KEY, tenant_id bigint, storage_backend_id text, vector_store_id text, deleted_at timestamptz)",
		"CREATE TABLE resources(id text PRIMARY KEY, storage_backend_id text, state text, deleted_at timestamptz)",
	} {
		require.NoError(t, db.Exec(sql).Error)
	}
	for _, entity := range []interface{ TableName() string }{&types.TenantSkillCatalogEntity{}, &types.TenantSkillEntity{}} {
		require.NoError(t, db.Exec("CREATE TABLE "+entity.TableName()+" (id text PRIMARY KEY, bundle_ref text, deleted_at timestamptz)").Error)
	}
	return db
}

func TestPlatformStorageVectorDefaultPostgres(t *testing.T) {
	for _, domain := range []string{"storage", "vector"} {
		t.Run(domain, func(t *testing.T) {
			db := platformStorageTestPostgres(t)
			ctx := context.Background()
			var setDefault func(context.Context, string) error
			var deleteRow func(context.Context, string) error
			table, binding := "storage_backends", "storage_backend_id"
			if domain == "storage" {
				svc := NewStorageBackendService(nil, db, nil)
				setDefault, deleteRow = svc.SetDefault, svc.Delete
				for _, id := range []string{"first", "second"} {
					require.NoError(t, db.Create(&types.StorageBackend{ID: id, Name: id, Provider: "local", Status: types.StorageBackendStatusActive}).Error)
				}
			} else {
				table, binding = "vector_stores", "vector_store_id"
				svc := NewVectorStoreService(nil, nil, nil, db)
				setDefault, deleteRow = svc.SetDefaultStore, svc.DeleteStore
				for _, id := range []string{"first", "second"} {
					require.NoError(t, db.Create(&types.VectorStore{ID: id, Name: id, EngineType: types.PostgresRetrieverEngineType}).Error)
				}
			}
			start := make(chan struct{})
			var wg sync.WaitGroup
			errs := make([]error, 2)
			for i, id := range []string{"first", "second"} {
				wg.Add(1)
				go func(i int, id string) { defer wg.Done(); <-start; errs[i] = setDefault(ctx, id) }(i, id)
			}
			close(start)
			wg.Wait()
			for _, err := range errs {
				require.NoError(t, err)
			}
			var count int64
			require.NoError(t, db.Table(table).Where("is_default AND deleted_at IS NULL").Count(&count).Error)
			require.Equal(t, int64(1), count)
			require.NoError(t, setDefault(ctx, "first"))
			require.Error(t, deleteRow(ctx, "first"))
			require.Error(t, setDefault(ctx, "missing"))
			require.NoError(t, db.Exec("INSERT INTO knowledge_bases(id,tenant_id,"+binding+") VALUES ('kb',10004,'second')").Error)
			require.Error(t, deleteRow(ctx, "second"), "another enterprise binding must prevent delete")
			require.NoError(t, db.Exec("DELETE FROM knowledge_bases WHERE id='kb'").Error)
			if domain == "storage" {
				ref := types.BuildStorageBackendPath("second", "local://platform/skills/catalog/example.zip")
				require.NoError(t, db.Exec("INSERT INTO "+(&types.TenantSkillCatalogEntity{}).TableName()+"(id,bundle_ref) VALUES ('catalog',?)", ref).Error)
				require.ErrorContains(t, deleteRow(ctx, "second"), "platform skill archive")
				require.NoError(t, db.Exec("DELETE FROM "+(&types.TenantSkillCatalogEntity{}).TableName()+" WHERE id='catalog'").Error)
				require.NoError(t, db.Model(&types.StorageBackend{}).Where("id = ?", "second").Update("status", types.StorageBackendStatusDisabled).Error)
				require.ErrorContains(t, setDefault(ctx, "second"), "active")
				require.NoError(t, db.Model(&types.StorageBackend{}).Where("id = ?", "second").Update("status", types.StorageBackendStatusActive).Error)
			}
			// Competing selection and retirement must leave an existing active default.
			start = make(chan struct{})
			errs = make([]error, 2)
			wg.Add(2)
			go func() { defer wg.Done(); <-start; errs[0] = setDefault(ctx, "second") }()
			go func() { defer wg.Done(); <-start; errs[1] = deleteRow(ctx, "second") }()
			close(start)
			wg.Wait()
			require.NotEqual(t, errs[0] == nil, errs[1] == nil, "exactly one conflicting operation can succeed")
			require.NoError(t, db.Table(table).Where("is_default AND deleted_at IS NULL").Count(&count).Error)
			require.Equal(t, int64(1), count)
		})
	}
}
