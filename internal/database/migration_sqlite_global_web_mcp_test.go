package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	sqlite3migrate "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/stretchr/testify/require"
)

func TestSQLiteGlobalWebAndMCPMigrationPreservesIDsAndOAuthScope(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)
	chdirAndRestore(t, repoRoot)
	dbPath := filepath.Join(t.TempDir(), "global-web-mcp.db")

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)
	driver, err := sqlite3migrate.WithInstance(db, &sqlite3migrate.Config{})
	require.NoError(t, err)
	m, err := migrate.NewWithDatabaseInstance("file://migrations/sqlite", "sqlite3", driver)
	require.NoError(t, err)
	require.NoError(t, m.Migrate(27))

	_, err = db.Exec(`INSERT INTO tenants(id, name, business, web_search_config)
		VALUES (7, 'tenant-7', '', '{"max_results":7}'),
		       (8, 'tenant-8', '', '{"max_results":8}')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO web_search_providers(id, tenant_id, name, provider, parameters, is_default)
		VALUES ('web-7', 7, 'web seven', 'keenable', '{}', 1),
		       ('web-8', 8, 'web eight', 'keenable', '{}', 1)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO mcp_services(id, tenant_id, name, transport_type, enabled)
		VALUES ('mcp-7', 7, 'mcp seven', 'http-streamable', 1)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO mcp_tool_approvals(id, tenant_id, service_id, tool_name, require_approval)
		VALUES ('approval-7', 7, 'mcp-7', 'write', 1)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO mcp_oauth_clients(id, tenant_id, service_id, client_id)
		VALUES ('client-7', 7, 'mcp-7', 'oauth-client-7')`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO mcp_oauth_tokens(
		id, tenant_id, user_id, principal_type, principal_id, service_id, access_token
	) VALUES ('token-7', 7, 'u1', 'web_user', 'u1', 'mcp-7', 'access-7')`)
	require.NoError(t, err)

	_, _ = m.Close()
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
	db = openSQLiteDB(t, dbPath)

	require.False(t, sqliteColumnExists(t, db, "tenants", "web_search_config"))
	require.False(t, sqliteColumnExists(t, db, "web_search_providers", "tenant_id"))
	require.False(t, sqliteColumnExists(t, db, "mcp_services", "tenant_id"))
	require.False(t, sqliteColumnExists(t, db, "mcp_tool_approvals", "tenant_id"))

	var count int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM web_search_providers
		WHERE id IN ('web-7', 'web-8') AND is_default = 0`).Scan(&count))
	require.Equal(t, 2, count)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM mcp_services WHERE id = 'mcp-7'`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM mcp_tool_approvals
		WHERE id = 'approval-7' AND service_id = 'mcp-7'`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM mcp_oauth_clients
		WHERE id = 'client-7' AND tenant_id = 7 AND service_id = 'mcp-7'`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM mcp_oauth_tokens
		WHERE id = 'token-7' AND tenant_id = 7 AND principal_id = 'u1' AND service_id = 'mcp-7'`).Scan(&count))
	require.Equal(t, 1, count)

	rows, err := db.Query(`PRAGMA foreign_key_check`)
	require.NoError(t, err)
	defer rows.Close()
	require.False(t, rows.Next(), "migration must not leave broken MCP foreign keys")
}
