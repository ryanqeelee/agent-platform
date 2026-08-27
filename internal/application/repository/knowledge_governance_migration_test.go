package repository

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func migrationSQL(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("..", "..", "..", path))
	require.NoError(t, err)
	return string(contents)
}

func requireKnowledgeAccessForeignKeys(t *testing.T, db *gorm.DB) {
	t.Helper()
	tenantInsert := `INSERT INTO tenants(id) VALUES (1), (2) ON CONFLICT DO NOTHING`
	kbInsert := `INSERT INTO knowledge_bases(id, tenant_id) VALUES ('kb-1', 1), ('kb-2', 2) ON CONFLICT DO NOTHING`
	if db.Dialector.Name() == "sqlite" {
		var tenantNameColumns int
		require.NoError(t, db.Raw(`SELECT count(*) FROM pragma_table_info('tenants') WHERE name = 'name'`).Scan(&tenantNameColumns).Error)
		if tenantNameColumns > 0 {
			tenantInsert = `INSERT INTO tenants(id, name, business) VALUES (1, 'one', 'retail'), (2, 'two', 'retail') ON CONFLICT DO NOTHING`
			kbInsert = `INSERT INTO knowledge_bases(id, name, tenant_id, embedding_model_id, summary_model_id) VALUES ('kb-1', 'one', 1, 'model', 'model'), ('kb-2', 'two', 2, 'model', 'model') ON CONFLICT DO NOTHING`
		}
	}
	require.NoError(t, db.Exec(tenantInsert).Error)
	require.NoError(t, db.Exec(kbInsert).Error)
	require.NoError(t, db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES ('employee', 1, 'viewer', 'active') ON CONFLICT DO NOTHING`).Error)
	require.NoError(t, db.Exec(`INSERT INTO business_roles(id, tenant_id, name) VALUES ('role-1', 1, 'one')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO business_role_members(tenant_id, role_id, tenant_member_id) SELECT 1, 'role-1', id FROM tenant_members WHERE tenant_id = 1 AND user_id = 'employee'`).Error)
	require.NoError(t, db.Exec(`INSERT INTO knowledge_base_role_grants(tenant_id, knowledge_base_id, role_id) VALUES (1, 'kb-1', 'role-1')`).Error)
	// Same IDs in another tenant must not pass either composite FK.
	require.Error(t, db.Exec(`INSERT INTO business_role_members(tenant_id, role_id, tenant_member_id) SELECT 2, 'role-1', id FROM tenant_members WHERE tenant_id = 1 AND user_id = 'employee'`).Error)
	require.Error(t, db.Exec(`INSERT INTO knowledge_base_role_grants(tenant_id, knowledge_base_id, role_id) VALUES (2, 'kb-2', 'role-1')`).Error)
}

func TestKnowledgeGovernanceSQLiteMigrationUpgradeAndFresh(t *testing.T) {
	open := func(t *testing.T, name string) *gorm.DB {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
		return db
	}
	t.Run("000004 upgrades an existing schema", func(t *testing.T) {
		db := open(t, "upgrade.db")
		require.NoError(t, db.Exec(`CREATE TABLE tenants (id INTEGER PRIMARY KEY)`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE knowledge_bases (id VARCHAR(36) PRIMARY KEY, tenant_id INTEGER NOT NULL)`).Error)
		require.NoError(t, db.Exec(`CREATE TABLE tenant_members (id INTEGER PRIMARY KEY, user_id VARCHAR(36) NOT NULL, tenant_id INTEGER NOT NULL, role VARCHAR(20) NOT NULL, status VARCHAR(20) NOT NULL, deleted_at DATETIME)`).Error)
		// This is the exact legacy production shape: an unconditional index
		// rejects a rejoin after soft deletion until 000004 replaces it.
		require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_tenant_members_user_tenant_unique ON tenant_members(user_id, tenant_id)`).Error)
		require.NoError(t, db.Exec(migrationSQL(t, "migrations/sqlite/000004_knowledge_access_roles.up.sql")).Error)
		requireKnowledgeAccessForeignKeys(t, db)

		require.NoError(t, db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES ('rejoin', 1, 'viewer', 'active')`).Error)
		var oldMembershipID int
		require.NoError(t, db.Raw(`SELECT id FROM tenant_members WHERE user_id = 'rejoin' AND deleted_at IS NULL`).Scan(&oldMembershipID).Error)
		require.NoError(t, db.Exec(`INSERT INTO business_role_members(tenant_id, role_id, tenant_member_id) VALUES (1, 'role-1', ?)`, oldMembershipID).Error)
		require.NoError(t, db.Exec(`UPDATE tenant_members SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?`, oldMembershipID).Error)
		require.NoError(t, db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES ('rejoin', 1, 'contributor', 'active')`).Error)
		var rejoinedMembershipID int
		require.NoError(t, db.Raw(`SELECT id FROM tenant_members WHERE user_id = 'rejoin' AND deleted_at IS NULL`).Scan(&rejoinedMembershipID).Error)
		require.NotEqual(t, oldMembershipID, rejoinedMembershipID, "rejoin must create a new membership identity")
		require.Error(t, db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES ('rejoin', 1, 'viewer', 'active')`).Error, "two active rows must be rejected")
		var inheritedRoles int
		require.NoError(t, db.Raw(`SELECT count(*) FROM business_role_members WHERE tenant_id = 1 AND tenant_member_id = ?`, rejoinedMembershipID).Scan(&inheritedRoles).Error)
		require.Zero(t, inheritedRoles, "rejoined membership must not inherit old business roles")

		require.NoError(t, db.Exec(migrationSQL(t, "migrations/sqlite/000004_knowledge_access_roles.down.sql")).Error)
		var memberships int
		require.NoError(t, db.Raw(`SELECT count(*) FROM tenant_members WHERE user_id = 'rejoin'`).Scan(&memberships).Error)
		require.Equal(t, 2, memberships, "down must preserve membership history")
		require.NoError(t, db.Exec(migrationSQL(t, "migrations/sqlite/000004_knowledge_access_roles.up.sql")).Error)
		var indexSQL string
		require.NoError(t, db.Raw(`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'idx_tenant_members_user_tenant_unique'`).Scan(&indexSQL).Error)
		require.Contains(t, strings.ToLower(indexSQL), "where deleted_at is null")
	})
	t.Run("000000 initializes a fresh SQLite database", func(t *testing.T) {
		db := open(t, "fresh.db")
		require.NoError(t, db.Exec(migrationSQL(t, "migrations/sqlite/000000_init.up.sql")).Error)
		requireKnowledgeAccessForeignKeys(t, db)
		require.NoError(t, db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES ('fresh-rejoin', 1, 'viewer', 'active')`).Error)
		var oldMembershipID int
		require.NoError(t, db.Raw(`SELECT id FROM tenant_members WHERE user_id = 'fresh-rejoin' AND deleted_at IS NULL`).Scan(&oldMembershipID).Error)
		require.NoError(t, db.Exec(`UPDATE tenant_members SET deleted_at = CURRENT_TIMESTAMP WHERE user_id = 'fresh-rejoin'`).Error)
		require.NoError(t, db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES ('fresh-rejoin', 1, 'contributor', 'active')`).Error)
		var rejoinedMembershipID int
		require.NoError(t, db.Raw(`SELECT id FROM tenant_members WHERE user_id = 'fresh-rejoin' AND deleted_at IS NULL`).Scan(&rejoinedMembershipID).Error)
		require.NotEqual(t, oldMembershipID, rejoinedMembershipID, "fresh schema rejoin must create a new membership identity")
		require.Error(t, db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES ('fresh-rejoin', 1, 'viewer', 'active')`).Error)
	})
}

func TestKnowledgeGovernancePostgresMigrationUpgrade(t *testing.T) {
	db := newKnowledgeGovernancePostgresDatabase(t)
	// This helper creates exactly the two pre-existing tables required by the
	// versioned upgrade; executing 000082 must add and constrain all three tables.
	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000082_knowledge_access_roles.up.sql")).Error)
	requireKnowledgeAccessForeignKeys(t, db)
	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000082_knowledge_access_roles.down.sql")).Error)
	require.NoError(t, db.Exec(migrationSQL(t, "migrations/versioned/000082_knowledge_access_roles.up.sql")).Error)
	requireKnowledgeAccessForeignKeys(t, db)
}
