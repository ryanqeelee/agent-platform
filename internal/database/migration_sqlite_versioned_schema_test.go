package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// versionedSQLiteTables is the set of tables that SQLite migrations must
// create to stay in sync with the versioned (PostgreSQL) migrations:
// 000041 task queue, 000053 system settings, 000055 processing spans,
// 000063 knowledge multi-tags.
var versionedSQLiteTables = []string{
	"task_pending_ops",
	"task_dead_letters",
	"system_settings",
	"knowledge_processing_spans",
	"knowledge_tag_relations",
	"memory_command_receipts",
	"memory_expressions",
	"platform_initial_administrator_receipts",
	"operating_brief_refreshes",
	"operating_brief_snapshots",
	"operating_brief_scope_refs",
	"platform_parser_config",
	"platform_memory_runtime_config",
	"platform_sandbox_configs",
	"platform_skill_catalog",
	"platform_skills",
	"platform_skill_snapshots",
	"platform_chat_history_config",
	"tenant_chat_history_indexes",
	"tenant_user_env_vars",
}

// versionedSQLiteColumns maps each existing table to the columns that the
// versioned migrations add and the SQLite baseline was missing.
var versionedSQLiteColumns = map[string][]string{
	"tenants":                  {"api_principal_config", "memory_generation", "seats_total", "governed_enterprise_id", "analysis_enabled", "ringxun_activation_idempotency_key_sha256", "ringxun_activation_plan_version_id", "ringxun_activation_completed_at", "ringxun_activation_last_error_code"},
	"users":                    {"is_system_admin"},                // 000053
	"knowledges":               {"pending_subtasks_count"},         // 000056
	"messages":                 {"attachments", "usage"},           // 000034, 000085
	"tenant_invitations":       {"token", "accepted_count"},        // 000054
	"embed_channels":           {"allow_memory"},                   // 000060
	"mcp_oauth_tokens":         {"principal_type", "principal_id"}, // 000064
	"memory_subjects":          {"generation", "revision"},         // 000101
	"memory_items":             {"scope"},                          // 000101
	"platform_sandbox_configs": {"is_default"},
	"platform_skills":          {"install_run_id", "install_transcript"},
}

const expectedSQLiteMigrationVersion = 36

func TestSQLiteMigrationsCreateVersionedSchema(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)
	chdirAndRestore(t, repoRoot)

	dbPath := filepath.Join(t.TempDir(), "fresh.db")
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))

	db := openSQLiteDB(t, dbPath)
	version, dirty := sqliteMigrationState(t, db)
	require.Equal(t, expectedSQLiteMigrationVersion, version)
	require.False(t, dirty)

	for _, table := range versionedSQLiteTables {
		require.Truef(t, sqliteTableExists(t, db, table), "SQLite migrations must create table %s", table)
	}
	for table, columns := range versionedSQLiteColumns {
		for _, column := range columns {
			require.Truef(
				t,
				sqliteColumnExists(t, db, table, column),
				"SQLite migrations must add column %s.%s",
				table,
				column,
			)
		}
	}

	assertSQLiteShareLinkInvitationsWork(t, db)
	assertSQLiteMCPOAuthPrincipalUpsertWorks(t, db)
	require.False(t, sqliteColumnExists(t, db, "knowledges", "tag_id"),
		"SQLite migrations must drop legacy knowledges.tag_id after multi-tag migration")
	require.False(t, sqliteColumnExists(t, db, "tenants", "parser_engine_config"),
		"SQLite migrations must move parser configuration out of tenants")
	require.False(t, sqliteColumnExists(t, db, "tenants", "chat_history_config"),
		"SQLite migrations must move chat-history configuration out of tenants")
	require.False(t, sqliteTableExists(t, db, "tenant_sandbox_configs"))
	require.False(t, sqliteTableExists(t, db, "tenant_skill_catalog"))
	require.False(t, sqliteTableExists(t, db, "tenant_skills"))
	require.False(t, sqliteTableExists(t, db, "tenant_skill_snapshots"))
	require.False(t, sqliteTableExists(t, db, "platform_skill_runs"))
}

func TestSQLiteMigrationsUpgradeV4PreservesData(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)

	// Build a legacy v4 migration root (000000_init .. 000004 access roles) so we
	// can prove the new migrations upgrade an existing Lite database without
	// replaying the baseline.
	legacyRoot := copySQLiteMigrationsV4(t, repoRoot)
	chdirAndRestore(t, legacyRoot)

	dbPath := filepath.Join(t.TempDir(), "upgrade.db")
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))

	db := openSQLiteDB(t, dbPath)
	versionBefore, dirtyBefore := sqliteMigrationState(t, db)
	require.Equal(t, 4, versionBefore)
	require.False(t, dirtyBefore)
	_, err := db.Exec("INSERT INTO tenants (name, business) VALUES (?, ?)", "upgrade-sentinel", "migration-test")
	require.NoError(t, err)
	_, err = db.Exec(
		"INSERT INTO knowledges (id, tenant_id, knowledge_base_id, type, title, source, tag_id) "+
			"VALUES (?, 1, ?, 'document', 'tagged-doc', 'manual', ?)",
		"legacy-knowledge-1", "legacy-kb-1", "legacy-tag-1",
	)
	require.NoError(t, err)

	// Run the full migration set from the repo root.
	chdirAndRestore(t, repoRoot)
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))

	db = openSQLiteDB(t, dbPath)
	versionAfter, dirtyAfter := sqliteMigrationState(t, db)
	require.Equal(t, expectedSQLiteMigrationVersion, versionAfter)
	require.False(t, dirtyAfter)

	for _, table := range versionedSQLiteTables {
		require.Truef(t, sqliteTableExists(t, db, table), "upgraded SQLite DB must have table %s", table)
	}
	for table, columns := range versionedSQLiteColumns {
		for _, column := range columns {
			require.Truef(
				t,
				sqliteColumnExists(t, db, table, column),
				"upgraded SQLite DB must have column %s.%s",
				table,
				column,
			)
		}
	}

	var sentinelName string
	require.NoError(t, db.QueryRow("SELECT name FROM tenants WHERE business = ?", "migration-test").Scan(&sentinelName))
	require.Equal(t, "upgrade-sentinel", sentinelName)

	var relationCount int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM knowledge_tag_relations WHERE knowledge_id = ? AND tag_id = ?",
		"legacy-knowledge-1", "legacy-tag-1",
	).Scan(&relationCount))
	require.Equal(t, 1, relationCount)
	require.False(t, sqliteColumnExists(t, db, "knowledges", "tag_id"))
	require.False(t, sqliteTableExists(t, db, "tenant_sandbox_configs"))
	require.False(t, sqliteTableExists(t, db, "platform_skill_runs"))
}

func TestSQLitePlatformChatHistoryMigrationRejectsUnsafeLegacyState(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)
	for _, test := range []struct {
		name   string
		legacy string
	}{
		{name: "bound knowledge base", legacy: `{"enabled":true,"embedding_model_id":"embed","knowledge_base_id":"legacy-kb"}`},
		{name: "malformed config", legacy: `{not-json`},
	} {
		t.Run(test.name, func(t *testing.T) {
			legacyRoot := copySQLiteMigrationsThrough(t, repoRoot, 34)
			chdirAndRestore(t, legacyRoot)
			dbPath := filepath.Join(t.TempDir(), "unsafe-chat-history.db")
			require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
			db := openSQLiteDB(t, dbPath)
			_, err := db.Exec(
				"INSERT INTO tenants (id, name, business, chat_history_config) VALUES (?, ?, ?, ?)",
				10001, "legacy", "migration-test", test.legacy,
			)
			require.NoError(t, err)

			chdirAndRestore(t, repoRoot)
			require.Error(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
			require.True(t, sqliteColumnExists(t, db, "tenants", "chat_history_config"))
			require.False(t, sqliteTableExists(t, db, "platform_chat_history_config"))
			require.False(t, sqliteTableExists(t, db, "tenant_chat_history_indexes"))
		})
	}
}

func TestSQLiteMemoryRuntimeMigrationPreservesTenantAndMemoryData(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)
	legacyRoot := copySQLiteMigrationsThrough(t, repoRoot, 34)
	chdirAndRestore(t, legacyRoot)
	dbPath := filepath.Join(t.TempDir(), "memory-runtime.db")
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
	db := openSQLiteDB(t, dbPath)

	legacyConfig := `{"enabled":true,"write_mode":"auto","extract_model_id":"","max_items":200,"extract_delay_seconds":90,"extract_min_interval_seconds":300,"extract_instructions":"","interest_threshold":3,"embedding_model_id":"","vector_recall":true,"retrieval_conditioning":null}`
	_, err := db.Exec(`
		INSERT INTO tenants(id, name, business, memory_config, memory_generation)
		VALUES (501, 'memory tenant', 'migration-test', ?, 7);
        INSERT INTO tenants(id, name, business) VALUES (502, 'unset tenant', 'migration-test');
        INSERT INTO tenants(id, name, business, memory_config) VALUES (503, 'disabled tenant', 'migration-test', '{"enabled":false,"write_mode":"explicit_only"}');
		INSERT INTO users(id, username, email, password_hash, tenant_id)
		VALUES ('memory-user', 'memory-user', 'memory@example.invalid', 'unused', 501);
		INSERT INTO knowledge_bases(id, name, tenant_id, embedding_model_id, summary_model_id)
		VALUES ('memory-kb', 'memory kb', 501, '', '');
		INSERT INTO memory_subjects(id, tenant_id, subject_id)
		VALUES ('memory-subject', 501, 'web_user:memory-user');
		INSERT INTO memory_items(id, tenant_id, subject_id, kind, content)
		VALUES ('memory-item', 501, 'web_user:memory-user', 'fact', 'preserved');
	`, legacyConfig)
	require.NoError(t, err)

	chdirAndRestore(t, repoRoot)
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
	version, dirty := sqliteMigrationState(t, db)
	require.Equal(t, expectedSQLiteMigrationVersion, version)
	require.False(t, dirty)

	var enabled bool
	var writeMode string
	var tenantGeneration int64
	var tenantConfigKeys, runtimeConfigKeys int
	require.NoError(t, db.QueryRow(`SELECT json_extract(memory_config, '$.enabled'),
		json_extract(memory_config, '$.write_mode'), memory_generation
		FROM tenants WHERE id = 501`).Scan(&enabled, &writeMode, &tenantGeneration))
	require.True(t, enabled)
	require.Equal(t, "auto", writeMode)
	require.Equal(t, int64(7), tenantGeneration)
	// Decode the stored JSON through the application scanner: json_extract
	// alone accepts SQLite numeric booleans and misses an unreadable DTO.
	var consent types.TenantMemoryConfig
	require.NoError(t, db.QueryRow("SELECT memory_config FROM tenants WHERE id = 501").Scan(&consent))
	require.True(t, consent.Enabled)
	require.Equal(t, "auto", consent.WriteMode)
	for _, id := range []int{502, 503} {
		var disabled types.TenantMemoryConfig
		require.NoError(t, db.QueryRow("SELECT memory_config FROM tenants WHERE id = ?", id).Scan(&disabled))
		require.False(t, disabled.Enabled)
	}

	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM json_each((SELECT memory_config FROM tenants WHERE id = 501))",
	).Scan(&tenantConfigKeys))
	require.Equal(t, 2, tenantConfigKeys)
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM json_each((SELECT runtime FROM platform_memory_runtime_config WHERE id = 1))",
	).Scan(&runtimeConfigKeys))
	require.Equal(t, 9, runtimeConfigKeys)

	for table, id := range map[string]string{
		"users": "memory-user", "knowledge_bases": "memory-kb",
		"memory_subjects": "memory-subject", "memory_items": "memory-item",
	} {
		var count int
		require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE id = ?", id).Scan(&count))
		require.Equalf(t, 1, count, "%s row was not preserved", table)
	}
}

func TestSQLiteMemoryRuntimeMigrationRejectsNonDefaultTenantRuntime(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)
	legacyRoot := copySQLiteMigrationsThrough(t, repoRoot, 34)
	chdirAndRestore(t, legacyRoot)
	dbPath := filepath.Join(t.TempDir(), "memory-runtime-reject.db")
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
	db := openSQLiteDB(t, dbPath)
	_, err := db.Exec(`INSERT INTO tenants(id, name, business, memory_config)
		VALUES (502, 'custom memory tenant', 'migration-test', '{"enabled":true,"max_items":201}')`)
	require.NoError(t, err)

	chdirAndRestore(t, repoRoot)
	err = RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath})
	require.Error(t, err)
	require.False(t, sqliteTableExists(t, db, "platform_memory_runtime_config"))
}

func TestSQLitePlatformSandboxMigrationPreservesIDsPinsAndDuplicateNames(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)
	legacyRoot := copySQLiteMigrationsThrough(t, repoRoot, 27)
	chdirAndRestore(t, legacyRoot)

	dbPath := filepath.Join(t.TempDir(), "platform-sandbox.db")
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
	db := openSQLiteDB(t, dbPath)
	_, err := db.Exec(`
		INSERT INTO tenants(id, name, business) VALUES
			(1, 'Tenant A', 'migration-test'),
			(2, 'Tenant B', 'migration-test');
		INSERT INTO tenant_sandbox_configs(
			id, tenant_id, name, sandbox_type, config
		) VALUES
			('config-a', 1, 'shared-name', 'e2b', '{}'),
			('config-b', 2, 'shared-name', 'e2b', '{}'),
			('policy-a', 1, '__workspace_scripts_policy__', 'e2b', '{}');
		INSERT INTO sessions(id, tenant_id, sandbox_config_id) VALUES
			('session-a', 1, 'config-a'),
			('session-b', 2, 'config-b');
	`)
	require.NoError(t, err)

	chdirAndRestore(t, repoRoot)
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))

	var configIDs []string
	rows, err := db.Query("SELECT id FROM platform_sandbox_configs ORDER BY id")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		configIDs = append(configIDs, id)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []string{"config-a", "config-b"}, configIDs)
	require.False(t, sqliteTableExists(t, db, "tenant_sandbox_configs"))
	require.False(t, sqliteColumnExists(t, db, "platform_sandbox_configs", "tenant_id"))

	var duplicateNames int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM platform_sandbox_configs WHERE name = 'shared-name'",
	).Scan(&duplicateNames))
	require.Equal(t, 2, duplicateNames)

	var tenantADisabled, tenantBDisabled bool
	require.NoError(t, db.QueryRow(
		"SELECT sandbox_scripts_disabled FROM tenants WHERE id = 1",
	).Scan(&tenantADisabled))
	require.NoError(t, db.QueryRow(
		"SELECT sandbox_scripts_disabled FROM tenants WHERE id = 2",
	).Scan(&tenantBDisabled))
	require.True(t, tenantADisabled)
	require.False(t, tenantBDisabled)

	var sessionAConfig, sessionBConfig string
	require.NoError(t, db.QueryRow(
		"SELECT sandbox_config_id FROM sessions WHERE id = 'session-a'",
	).Scan(&sessionAConfig))
	require.NoError(t, db.QueryRow(
		"SELECT sandbox_config_id FROM sessions WHERE id = 'session-b'",
	).Scan(&sessionBConfig))
	require.Equal(t, "config-a", sessionAConfig)
	require.Equal(t, "config-b", sessionBConfig)
}

func TestSQLitePlatformIdentityMigrationNormalizesLegacyMixedIdentity(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)
	legacyRoot := copySQLiteMigrationsThrough(t, repoRoot, 23)
	chdirAndRestore(t, legacyRoot)

	dbPath := filepath.Join(t.TempDir(), "platform-identity.db")
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
	db := openSQLiteDB(t, dbPath)
	_, err := db.Exec("INSERT INTO tenants (name, business) VALUES (?, ?)", "Acme", "migration-test")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO users
		(id, username, email, password_hash, tenant_id, is_active, can_access_all_tenants, is_system_admin)
		VALUES ('legacy-platform', 'legacy-platform', 'legacy@example.invalid', 'unused', 1, 1, 1, 1)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO tenant_members (user_id, tenant_id, role, status)
		VALUES ('legacy-platform', 1, 'admin', 'active')`)
	require.NoError(t, err)

	chdirAndRestore(t, repoRoot)
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))

	var tenantID sql.NullInt64
	var crossTenant bool
	require.NoError(t, db.QueryRow(
		"SELECT tenant_id, can_access_all_tenants FROM users WHERE id = 'legacy-platform'",
	).Scan(&tenantID, &crossTenant))
	require.False(t, tenantID.Valid)
	require.False(t, crossTenant)
	var deletedAt sql.NullString
	require.NoError(t, db.QueryRow(
		"SELECT deleted_at FROM tenant_members WHERE user_id = 'legacy-platform'",
	).Scan(&deletedAt))
	require.True(t, deletedAt.Valid)
}

func sqliteRepoRoot(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return repoRoot
}

func chdirAndRestore(t *testing.T, dir string) {
	t.Helper()
	previousDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(previousDir) })
}

func openSQLiteDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func sqliteMigrationState(t *testing.T, db *sql.DB) (version int, dirty bool) {
	t.Helper()
	require.NoError(t, db.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty))
	return version, dirty
}

func sqliteTableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
		table,
	).Scan(&n))
	return n == 1
}

func sqliteColumnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?",
		table,
		column,
	).Scan(&n))
	return n == 1
}

func assertSQLiteShareLinkInvitationsWork(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec("INSERT INTO tenants (name, business) VALUES (?, ?)", "share-link-tenant", "share-link-test")
	require.NoError(t, err)

	expiresAt := "2099-01-01 00:00:00"
	shareLinkInsert := "INSERT INTO tenant_invitations " +
		"(tenant_id, invitee_user_id, token, role, status, expires_at) " +
		"VALUES (1, '', ?, 'member', 'pending', ?)"
	_, err = db.Exec(shareLinkInsert, "token-a", expiresAt)
	require.NoError(t, err)
	_, err = db.Exec(shareLinkInsert, "token-b", expiresAt)
	require.NoError(t, err)

	var count int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM tenant_invitations WHERE tenant_id = 1 AND invitee_user_id = '' AND status = 'pending'",
	).Scan(&count))
	require.Equal(t, 2, count)
}

func assertSQLiteMCPOAuthPrincipalUpsertWorks(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO mcp_services (id, name, transport_type) VALUES (?, 'svc', 'http')",
		"svc-migration-1",
	)
	require.NoError(t, err)

	tokenInsertPrefix := "INSERT INTO mcp_oauth_tokens " +
		"(id, tenant_id, user_id, service_id, principal_type, principal_id, access_token) "
	_, err = db.Exec(
		tokenInsertPrefix +
			"VALUES ('tok-1', 1, 'u1', 'svc-migration-1', 'web_user', 'u1', 'token-1')",
	)
	require.NoError(t, err)

	_, err = db.Exec(
		tokenInsertPrefix +
			"VALUES ('tok-2', 1, 'u1', 'svc-migration-1', 'web_user', 'u1', 'token-2') " +
			"ON CONFLICT(tenant_id, principal_type, principal_id, service_id) " +
			"DO UPDATE SET access_token = excluded.access_token",
	)
	require.NoError(t, err)

	var accessToken string
	require.NoError(t, db.QueryRow(
		"SELECT access_token FROM mcp_oauth_tokens "+
			"WHERE tenant_id = 1 AND principal_type = 'web_user' "+
			"AND principal_id = 'u1' AND service_id = 'svc-migration-1'",
	).Scan(&accessToken))
	require.Equal(t, "token-2", accessToken)

	var rowCount int
	require.NoError(t, db.QueryRow(
		"SELECT COUNT(*) FROM mcp_oauth_tokens WHERE tenant_id = 1 AND service_id = 'svc-migration-1'",
	).Scan(&rowCount))
	require.Equal(t, 1, rowCount)
}

func copySQLiteMigrationsV4(t *testing.T, repoRoot string) string {
	t.Helper()
	dest := t.TempDir()
	srcDir := filepath.Join(repoRoot, "migrations", "sqlite")
	destDir := filepath.Join(dest, "migrations", "sqlite")
	require.NoError(t, os.MkdirAll(destDir, 0o755))

	legacy := []string{
		"000000_init.up.sql",
		"000001_remove_wiki_log.up.sql",
		"000002_knowledge_folder_path.up.sql",
		"000003_revoke_legacy_owner_invitations.up.sql",
		"000004_knowledge_access_roles.up.sql",
	}
	for _, name := range legacy {
		data, err := os.ReadFile(filepath.Join(srcDir, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(destDir, name), data, 0o600))
	}
	return dest
}

func copySQLiteMigrationsThrough(t *testing.T, repoRoot string, maxVersion int) string {
	t.Helper()
	dest := t.TempDir()
	srcDir := filepath.Join(repoRoot, "migrations", "sqlite")
	destDir := filepath.Join(dest, "migrations", "sqlite")
	require.NoError(t, os.MkdirAll(destDir, 0o755))
	entries, err := os.ReadDir(srcDir)
	require.NoError(t, err)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".up.sql") || len(name) < 6 {
			continue
		}
		version, err := strconv.Atoi(name[:6])
		if err != nil || version > maxVersion {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, name))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(destDir, name), data, 0o600))
	}
	return dest
}
