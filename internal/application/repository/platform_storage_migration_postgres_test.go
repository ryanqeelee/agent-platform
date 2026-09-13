package repository

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPlatformStorageVectorMigrationPostgres(t *testing.T) {
	db := newIsolatedPostgresTestDatabase(t, "platform_storage_migration_")
	for _, sql := range []string{
		"CREATE TABLE tenants(id bigint PRIMARY KEY, storage_engine_config jsonb)",
		"CREATE TABLE knowledge_bases(id text PRIMARY KEY, tenant_id bigint, vector_store_id varchar(36), deleted_at timestamptz)",
		migrationSQL(t, "migrations/versioned/000032_vector_stores.up.sql"),
		migrationSQL(t, "migrations/versioned/000068_storage_backends.up.sql"),
		"INSERT INTO tenants(id,default_storage_backend_id) VALUES(10001,'s1'),(10004,'s2')",
		"INSERT INTO storage_backends(id,tenant_id,name,provider) VALUES('s1',10001,'Local','local'),('s2',10004,'Local','local')",
		"INSERT INTO vector_stores(id,tenant_id,name,engine_type) VALUES('v1',10001,'Postgres','postgres'),('v2',10004,'Postgres','postgres')",
		"INSERT INTO knowledge_bases(id,tenant_id,storage_backend_id,vector_store_id) VALUES('kb1',10001,NULL,'v1'),('kb2',10004,'s2','v2')",
		migrationSQL(t, "migrations/versioned/000108_global_storage_backends.up.sql"),
		migrationSQL(t, "migrations/versioned/000109_global_vector_stores.up.sql"),
	} {
		require.NoError(t, db.Exec(sql).Error)
	}
	for _, table := range []string{"storage_backends", "vector_stores"} {
		var count int64
		require.NoError(t, db.Table(table).Count(&count).Error)
		require.Equal(t, int64(2), count)
		require.NoError(t, db.Table(table).Where("is_default").Count(&count).Error)
		require.Zero(t, count)
		require.NoError(t, db.Raw("SELECT count(DISTINCT name) FROM "+table).Scan(&count).Error)
		require.Equal(t, int64(2), count)
	}
	type binding struct {
		ID               string
		TenantID         uint64
		StorageBackendID string
		VectorStoreID    string
	}
	var rows []binding
	require.NoError(t, db.Table("knowledge_bases").Order("id").Find(&rows).Error)
	require.Equal(t, []binding{{"kb1", 10001, "s1", "v1"}, {"kb2", 10004, "s2", "v2"}}, rows)
	for _, field := range []struct{ table, column string }{
		{"storage_backends", "tenant_id"}, {"storage_backends", "legacy_alias"},
		{"vector_stores", "tenant_id"}, {"tenants", "storage_engine_config"}, {"tenants", "default_storage_backend_id"},
	} {
		var count int64
		require.NoError(t, db.Raw("SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=? AND column_name=?", field.table, field.column).Scan(&count).Error)
		require.Zero(t, count, "obsolete ownership field %s.%s", field.table, field.column)
	}
}
