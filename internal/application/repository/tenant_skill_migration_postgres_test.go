package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestPlatformSkillMigrationMovesRowsWithoutLegacyTablesOrIdentityLoss(t *testing.T) {
	db := newIsolatedPostgresTestDatabase(t, "weknora_platform_skills_")
	for _, migration := range []string{
		"migrations/versioned/000093_tenant_skills.up.sql",
		"migrations/versioned/000094_skill_install_transcript.up.sql",
		"migrations/versioned/000095_skill_snapshot_planned_name.up.sql",
		"migrations/versioned/000096_env_vars.up.sql",
		"migrations/versioned/000097_skill_catalog.up.sql",
	} {
		require.NoError(t, db.Exec(migrationSQL(t, migration)).Error)
	}
	require.NoError(t, db.Exec(`
		CREATE TABLE sessions (id VARCHAR(36) PRIMARY KEY, tenant_id BIGINT NOT NULL);
		INSERT INTO sessions(id, tenant_id) VALUES ('session-a', 1), ('session-b', 2);
		INSERT INTO tenant_skill_catalog(
			id, tenant_id, name, bundle_ref, bundle_sha256
		) VALUES
			('cat-a', 1, 'same-name', 'storage://platform/skills/a.zip', 'sha-a'),
			('cat-b', 2, 'same-name', 'storage://platform/skills/b.zip', 'sha-b');
		INSERT INTO tenant_skills(
			id, tenant_id, sandbox_config_id, catalog_id, name, enabled, status,
			install_session_id, install_message_id
		) VALUES
			('skill-a', 1, 'config-a', 'cat-a', 'same-name', TRUE, 'ready', 'session-a', 'message-a'),
			('skill-b', 2, 'config-b', 'cat-b', 'same-name', TRUE, 'ready', 'session-b', 'message-b');
		INSERT INTO tenant_skill_snapshots(
			id, tenant_id, sandbox_config_id, skill_id, snapshot_id, trigger, state
		) VALUES ('snapshot-a', 1, 'config-a', 'skill-a', 'provider-snapshot-a', 'install', 'active');
		INSERT INTO tenant_user_env_vars(
			id, tenant_id, principal_type, principal_id, sandbox_config_id, skill_id, name, value
		) VALUES ('env-a', 1, 'web_user', 'user-a', 'config-a', 'skill-a', 'TOKEN', 'encrypted');
	`).Error)

	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000113_platform_skills.up.sql")).Error)

	var catalogIDs, skillIDs, snapshotIDs, envIDs, sessionIDs []string
	require.NoError(t, db.Raw(`SELECT id FROM platform_skill_catalog ORDER BY id`).Scan(&catalogIDs).Error)
	require.Equal(t, []string{"cat-a", "cat-b"}, catalogIDs)
	require.NoError(t, db.Raw(`SELECT id FROM platform_skills ORDER BY id`).Scan(&skillIDs).Error)
	require.Equal(t, []string{"skill-a", "skill-b"}, skillIDs)
	require.NoError(t, db.Raw(`SELECT id FROM platform_skill_snapshots ORDER BY id`).Scan(&snapshotIDs).Error)
	require.Equal(t, []string{"snapshot-a"}, snapshotIDs)
	require.NoError(t, db.Raw(`SELECT id FROM tenant_user_env_vars ORDER BY id`).Scan(&envIDs).Error)
	require.Equal(t, []string{"env-a"}, envIDs)
	require.NoError(t, db.Raw(`SELECT id FROM sessions ORDER BY id`).Scan(&sessionIDs).Error)
	require.Equal(t, []string{"session-a", "session-b"}, sessionIDs)

	var oldTables, runTables int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM pg_tables
		WHERE schemaname = current_schema()
		  AND tablename IN ('tenant_skills', 'tenant_skill_catalog', 'tenant_skill_snapshots')`).Scan(&oldTables).Error)
	require.Zero(t, oldTables)
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM pg_tables
		WHERE schemaname = current_schema() AND tablename = 'platform_skill_runs'`).Scan(&runTables).Error)
	require.Zero(t, runTables)

	var tenantColumns, populatedRunFields int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name IN ('platform_skills', 'platform_skill_catalog', 'platform_skill_snapshots')
		  AND column_name = 'tenant_id'`).Scan(&tenantColumns).Error)
	require.Zero(t, tenantColumns)
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM platform_skills
		WHERE install_run_id <> '' OR install_transcript IS NOT NULL`).Scan(&populatedRunFields).Error)
	require.Zero(t, populatedRunFields, "historical business transcript locators are not platform run records")

	repo := NewTenantSkillRepository(db)
	ctx := context.Background()
	row, err := repo.GetSkill(ctx, "config-a", "skill-a")
	require.NoError(t, err)
	require.Empty(t, row.InstallRunID)
	row.Description = "attached without a run"
	matched, err := repo.UpdateSkillForRun(ctx, row, "")
	require.NoError(t, err)
	require.True(t, matched, "a migrated ready row must match the empty run token")

	require.NoError(t, repo.BeginSkillRun(
		ctx, "config-a", "skill-a", "run-current", types.SkillStatusInstalling, time.Now(),
	))
	row.Description = "stale empty-token write"
	matched, err = repo.UpdateSkillForRun(ctx, row, "")
	require.NoError(t, err)
	require.False(t, matched)
	current, err := repo.GetSkill(ctx, "config-a", "skill-a")
	require.NoError(t, err)
	current.Description = "current-token write"
	matched, err = repo.UpdateSkillForRun(ctx, current, "run-current")
	require.NoError(t, err)
	require.True(t, matched)
}
