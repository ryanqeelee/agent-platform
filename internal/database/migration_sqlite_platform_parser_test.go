package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSQLitePlatformParserMigrationCopiesEquivalentConfigAndPreservesTenants(t *testing.T) {
	repoRoot := sqliteRepoRoot(t)
	legacyRoot := copySQLiteMigrationsThrough(t, repoRoot, 27)
	chdirAndRestore(t, legacyRoot)
	dbPath := filepath.Join(t.TempDir(), "parser-equivalent.db")
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
	db := openSQLiteDB(t, dbPath)
	_, err := db.Exec("INSERT INTO tenants (id, name, business, parser_engine_config) VALUES (?, ?, ?, ?)",
		42, "tenant-a", "parser-test", `{"mineru_endpoint":"https://mineru.example.invalid","chat_parser_engine_rules":[{"file_types":["pdf"],"engine":"mineru"}]}`)
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO tenants (id, name, business, parser_engine_config) VALUES (?, ?, ?, ?)",
		43, "tenant-b", "parser-test", `{"chat_parser_engine_rules":[{"engine":"mineru","file_types":["pdf"]}],"mineru_endpoint":"https://mineru.example.invalid"}`)
	require.NoError(t, err)

	chdirAndRestore(t, repoRoot)
	require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
	db = openSQLiteDB(t, dbPath)
	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM tenants WHERE id IN (42, 43)").Scan(&count))
	require.Equal(t, 2, count)
	require.False(t, sqliteColumnExists(t, db, "tenants", "parser_engine_config"))
	var config string
	require.NoError(t, db.QueryRow("SELECT config FROM platform_parser_config WHERE id = 1").Scan(&config))
	require.Contains(t, config, "chat_parser_engine_rules")
}

func TestSQLitePlatformParserMigrationRejectsConflictAndPlaintextSecrets(t *testing.T) {
	for _, test := range []struct {
		name   string
		first  string
		second string
	}{
		{name: "different configs", first: `{"mineru_endpoint":"https://a.invalid"}`, second: `{"mineru_endpoint":"https://b.invalid"}`},
		{name: "plaintext credential", first: `{"mineru_api_key":"legacy-secret"}`, second: `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			repoRoot := sqliteRepoRoot(t)
			legacyRoot := copySQLiteMigrationsThrough(t, repoRoot, 27)
			chdirAndRestore(t, legacyRoot)
			dbPath := filepath.Join(t.TempDir(), "parser-reject.db")
			require.NoError(t, RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath}))
			db := openSQLiteDB(t, dbPath)
			_, err := db.Exec("INSERT INTO tenants (id, name, business, parser_engine_config) VALUES (?, ?, ?, ?)", 42, "tenant-a", "parser-test", test.first)
			require.NoError(t, err)
			_, err = db.Exec("INSERT INTO tenants (id, name, business, parser_engine_config) VALUES (?, ?, ?, ?)", 43, "tenant-b", "parser-test", test.second)
			require.NoError(t, err)

			chdirAndRestore(t, repoRoot)
			err = RunMigrationsWithOptions("sqlite3://unused", MigrationOptions{SQLiteDBPath: dbPath})
			require.Error(t, err)
			db, openErr := sql.Open("sqlite3", dbPath)
			require.NoError(t, openErr)
			defer db.Close()
			var count int
			require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM tenants WHERE id IN (42, 43)").Scan(&count))
			require.Equal(t, 2, count)
		})
	}
}
