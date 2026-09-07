package database

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	sqlite3migrate "github.com/golang-migrate/migrate/v4/database/sqlite3"
)

func TestRunMigrations_SQLiteDiscoversLegacyOwnerInvitationRevocation(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	t.Chdir(root)
	path := filepath.Join(t.TempDir(), "lite.db")

	// Establish a deployed Lite database at version 2, rather than exercising
	// only a fresh init where 000000 already has the current schema.
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	driver, err := sqlite3migrate.WithInstance(db, &sqlite3migrate.Config{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://migrations/sqlite", "sqlite3", driver)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Steps(3); err != nil {
		t.Fatal(err)
	}
	_, _ = m.Close()
	db, err = sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`INSERT INTO tenant_invitations(tenant_id, invitee_user_id, role, status, expires_at) VALUES (1, 'legacy', 'owner', 'pending', CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := RunMigrationsWithOptions("sqlite3://"+path, MigrationOptions{SQLiteDBPath: path}); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var status string
	if err := db.QueryRow(`SELECT status FROM tenant_invitations WHERE invitee_user_id = 'legacy'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "revoked" {
		t.Fatalf("legacy Owner invitation status = %q, want revoked", status)
	}
	if _, err := db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES ('admin-a', 1, 'admin', 'active'), ('admin-b', 1, 'admin', 'active')`); err != nil {
		t.Fatal("two-role migration should allow multiple administrators")
	}
	if _, err := db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES ('employee-a', 1, 'viewer', 'active')`); err != nil {
		t.Fatal(err)
	}
	var operatingAnalysisAccess bool
	if err := db.QueryRow(`SELECT operating_analysis_access FROM tenant_members WHERE user_id = 'employee-a'`).Scan(&operatingAnalysisAccess); err != nil || operatingAnalysisAccess {
		t.Fatalf("operating analysis default = %v, err=%v", operatingAnalysisAccess, err)
	}
}

func TestRunMigrations_SQLiteEnterpriseActivationReceiptUpDown(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	t.Chdir(root)
	path := filepath.Join(t.TempDir(), "activation.db")

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	driver, err := sqlite3migrate.WithInstance(db, &sqlite3migrate.Config{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://migrations/sqlite", "sqlite3", driver)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Migrate(7); err != nil {
		t.Fatal(err)
	}

	assertReceiptUnique := func() {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO tenants(name, business, ringxun_activation_id) VALUES ('first', '', 'receipt-1')`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO tenants(name, business, ringxun_activation_id) VALUES ('second', '', 'receipt-1')`); err == nil {
			t.Fatal("activation receipt unique index accepted a duplicate")
		}
	}
	assertReceiptUnique()

	if err := m.Migrate(5); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT ringxun_activation_id FROM tenants LIMIT 1`); err == nil {
		t.Fatal("activation receipt down migration left its columns behind")
	}
	if err := m.Migrate(7); err != nil {
		t.Fatal(err)
	}
	assertReceiptUnique()
}
