package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPlatformSandboxDefaultPostgresConcurrentSelectionAndDeletion(t *testing.T) {
	db := newIsolatedPostgresTestDatabase(t, "weknora_sandbox_default_")
	require.NoError(t, db.Exec(`CREATE TABLE tenants (id BIGINT PRIMARY KEY)`).Error)
	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000089_tenant_sandbox_config.up.sql")).Error)
	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000112_platform_sandbox_configs.up.sql")).Error)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repo := NewTenantSandboxConfigRepository(db)
	for _, id := range []string{"a", "b"} {
		require.NoError(t, db.Exec(`INSERT INTO platform_sandbox_configs (id,name,sandbox_type,config) VALUES (?,?,'e2b','{}')`, id, id).Error)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, id := range []string{"a", "b"} {
		go func(id string) { <-start; results <- repo.SetDefault(ctx, id) }(id)
	}
	close(start)
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	var count int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM platform_sandbox_configs WHERE is_default AND deleted_at IS NULL`).Scan(&count).Error)
	require.EqualValues(t, 1, count)

	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("race-%d", i)
		require.NoError(t, db.Exec(`INSERT INTO platform_sandbox_configs (id,name,sandbox_type,config) VALUES (?,?,'e2b','{}')`, id, id).Error)
		gate := make(chan struct{})
		selected, deleted := make(chan error, 1), make(chan error, 1)
		go func() { <-gate; selected <- repo.SetDefault(ctx, id) }()
		go func() { <-gate; deleted <- repo.SoftDelete(ctx, id) }()
		close(gate)
		selectErr, deleteErr := <-selected, <-deleted
		if selectErr == nil {
			require.ErrorIs(t, deleteErr, ErrDeleteDefaultSandboxConfig)
			current, err := repo.GetDefault(ctx)
			require.NoError(t, err)
			require.Equal(t, id, current.ID)
		} else {
			require.NoError(t, deleteErr)
			row, err := repo.GetByID(ctx, id)
			require.NoError(t, err)
			require.Nil(t, row)
		}
	}
}

func TestPlatformSandboxMigrationPostgresPreservesPinsAndEnterprisePolicy(t *testing.T) {
	db := newIsolatedPostgresTestDatabase(t, "weknora_platform_sandbox_")
	require.NoError(t, db.Exec(`CREATE TABLE tenants (id BIGINT PRIMARY KEY);
		INSERT INTO tenants VALUES (1), (2);
		CREATE TABLE sessions (id TEXT PRIMARY KEY, tenant_id BIGINT, sandbox_config_id TEXT);
		INSERT INTO sessions VALUES ('session-a', 1, 'config-a'), ('session-b', 2, 'config-b');`).Error)
	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000089_tenant_sandbox_config.up.sql")).Error)
	require.NoError(t, db.Exec(`INSERT INTO tenant_sandbox_configs
		(id, tenant_id, name, sandbox_type, config) VALUES
		('config-a', 1, 'shared-name', 'e2b', '{"api_key":"encrypted-a"}'),
		('config-b', 2, 'shared-name', 'e2b', '{"api_key":"encrypted-b"}'),
		('policy-a', 1, '__workspace_scripts_policy__', 'policy', '{}');`).Error)
	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000112_platform_sandbox_configs.up.sql")).Error)
	var ids []string
	require.NoError(t, db.Raw(`SELECT id FROM platform_sandbox_configs ORDER BY id`).Scan(&ids).Error)
	require.Equal(t, []string{"config-a", "config-b"}, ids)
	var preserved int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM sessions s JOIN platform_sandbox_configs c
		ON c.id = s.sandbox_config_id WHERE c.config->>'api_key' = 'encrypted-' || RIGHT(s.id, 1)`).Scan(&preserved).Error)
	require.EqualValues(t, 2, preserved)
	var disabled []int64
	require.NoError(t, db.Raw(`SELECT id FROM tenants WHERE sandbox_scripts_disabled ORDER BY id`).Scan(&disabled).Error)
	require.Equal(t, []int64{1}, disabled)
	var defaults int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM platform_sandbox_configs WHERE is_default`).Scan(&defaults).Error)
	require.Zero(t, defaults, "the platform default must be explicitly selected")
	var retired int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'platform_sandbox_configs' AND column_name = 'tenant_id'`).Scan(&retired).Error)
	require.Zero(t, retired)
}
