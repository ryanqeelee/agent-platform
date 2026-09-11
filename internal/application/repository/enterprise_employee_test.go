package repository

import (
	"context"
	"errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"sync"
	"testing"
	"time"
)

// Create path matrix: active own administrator -> employee; viewer, suspended,
// foreign, missing actor -> no writes; duplicate identity -> conflict/no overwrite;
// membership insert failure -> no user; concurrent duplicate -> one account;
// administrator revocation committed while waiting -> no account.
func TestEnterpriseEmployeePostgres(t *testing.T) {
	dsn := os.Getenv("WEKNORA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("WEKNORA_TEST_POSTGRES_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatal(err)
	}
	// This suite owns a fresh schema in the isolated acceptance database.
	schema := "employee_" + uuid.NewString()[:8]
	if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DROP SCHEMA " + schema + " CASCADE") })
	db, err = gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE tenants (id bigint primary key, status text NOT NULL DEFAULT 'active', seats_total integer, deleted_at timestamptz)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&types.User{}, &types.TenantMember{}); err != nil {
		t.Fatal(err)
	}
	db.Exec("INSERT INTO tenants(id) VALUES (1),(2)")
	for _, seed := range []struct {
		id     string
		tenant uint64
		role   types.TenantRole
		status types.TenantMemberStatus
	}{
		{"admin", 1, types.TenantRoleAdmin, types.TenantMemberStatusActive},
		{"viewer", 1, types.TenantRoleViewer, types.TenantMemberStatusActive},
		{"suspended", 1, types.TenantRoleAdmin, types.TenantMemberStatusSuspended},
		{"foreign", 2, types.TenantRoleAdmin, types.TenantMemberStatusActive},
	} {
		if err := db.Create(&types.User{ID: seed.id, Username: seed.id, Email: seed.id + "@example.invalid", TenantID: seed.tenant, PasswordHash: "unchanged", IsActive: true}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&types.TenantMember{UserID: seed.id, TenantID: seed.tenant, Role: seed.role, Status: seed.status}).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := &tenantMemberRepository{db: db}
	create := func(actor, name string) (*types.User, error) {
		u := &types.User{ID: uuid.NewString(), Username: name, Email: name + "@example.invalid", PasswordHash: "new-hash", TenantID: 1, IsActive: true}
		m := &types.TenantMember{UserID: u.ID, TenantID: 1, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive}
		return u, repo.CreateEmployee(context.Background(), types.MemberActorAuthority{UserID: actor}, u, m)
	}
	for _, actor := range []string{"viewer", "suspended", "foreign", "missing", ""} {
		t.Run("deny-"+actor, func(t *testing.T) {
			u, err := create(actor, "denied-"+actor)
			if !errors.Is(err, ErrMemberActionForbidden) {
				t.Fatalf("err=%v", err)
			}
			var n int64
			db.Model(&types.User{}).Where("id = ?", u.ID).Count(&n)
			if n != 0 {
				t.Fatal("unauthorized account persisted")
			}
		})
	}
	t.Run("creates-bound-employee", func(t *testing.T) {
		u, err := create("admin", "new-employee")
		if err != nil {
			t.Fatal(err)
		}
		m, err := repo.Get(context.Background(), u.ID, 1)
		if err != nil || m == nil || m.Role != types.TenantRoleViewer || m.OperatingAnalysisAccess {
			t.Fatalf("membership=%+v err=%v", m, err)
		}
	})
	t.Run("duplicate-preserves-password", func(t *testing.T) {
		_, err := create("admin", "foreign")
		if err == nil {
			t.Fatal("duplicate accepted")
		}
		var u types.User
		db.First(&u, "id = ?", "foreign")
		if u.PasswordHash != "unchanged" || u.TenantID != 2 {
			t.Fatal("existing identity changed")
		}
	})
	t.Run("rollback-on-membership-failure", func(t *testing.T) {
		u := &types.User{ID: uuid.NewString(), Username: "rollback", Email: "rollback@example.invalid", TenantID: 1, PasswordHash: "hash"}
		m := &types.TenantMember{UserID: "missing-target", TenantID: 1, Role: types.TenantRoleViewer}
		if err := repo.CreateEmployee(context.Background(), types.MemberActorAuthority{UserID: "admin"}, u, m); err == nil {
			t.Fatal("expected membership failure")
		}
		var n int64
		db.Model(&types.User{}).Where("id = ?", u.ID).Count(&n)
		if n != 0 {
			t.Fatal("orphan user")
		}
	})
	t.Run("concurrent-duplicate", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := create("admin", "concurrent"); results <- err }()
		}
		wg.Wait()
		close(results)
		success := 0
		for err := range results {
			if err == nil {
				success++
			} else if !errors.Is(err, gorm.ErrDuplicatedKey) {
				t.Fatal(err)
			}
		}
		if success != 1 {
			t.Fatalf("success=%d", success)
		}
	})
	t.Run("revocation-before-create", func(t *testing.T) {
		tx := db.Begin()
		if err := tx.Exec("SELECT id FROM tenants WHERE id=1 FOR UPDATE").Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Model(&types.TenantMember{}).Where("user_id = ?", "admin").Update("status", types.TenantMemberStatusSuspended).Error; err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() { _, err := create("admin", "revoked"); result <- err }()
		select {
		case err := <-result:
			tx.Rollback()
			t.Fatalf("creation bypassed revocation: %v", err)
		case <-time.After(150 * time.Millisecond):
		}
		if err := tx.Commit().Error; err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-result:
			if !errors.Is(err, ErrMemberActionForbidden) {
				t.Fatalf("err=%v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("creation did not finish")
		}
	})
}
