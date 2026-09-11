package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestUpdateUserCannotPromoteEnterpriseIdentity(t *testing.T) {
	db := activationTestDB(t)
	tenant := &types.Tenant{Name: "Acme", Status: types.TenantStatusActive}
	if err := db.Create(tenant).Error; err != nil {
		t.Fatal(err)
	}
	user := &types.User{
		ID: "member", Username: "member", Email: "member@example.invalid",
		PasswordHash: "unused", TenantID: tenant.ID, IsActive: true,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&types.TenantMember{
		UserID: user.ID, TenantID: tenant.ID, Role: types.TenantRoleViewer,
		Status: types.TenantMemberStatusSuspended,
	}).Error; err != nil {
		t.Fatal(err)
	}
	user.IsSystemAdmin = true
	err := (&userRepository{db: db}).UpdateUser(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	var stored types.User
	if err := db.First(&stored, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.IsSystemAdmin {
		t.Fatal("enterprise identity was promoted")
	}
}

func TestPromoteSystemAdminPromotesOnlyTenantlessIdentity(t *testing.T) {
	db := activationTestDB(t)
	user := &types.User{
		ID: "operator", Username: "operator", Email: "operator@example.invalid",
		PasswordHash: "unused", IsActive: true,
	}
	if err := db.Omit("TenantID").Create(user).Error; err != nil {
		t.Fatal(err)
	}
	user.IsSystemAdmin = true
	if err := (&userRepository{db: db}).UpdateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	var unchanged types.User
	if err := db.First(&unchanged, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.IsSystemAdmin {
		t.Fatal("general update granted platform authority")
	}
	if _, err := (&userRepository{db: db}).PromoteSystemAdmin(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
	var stored types.User
	if err := db.First(&stored, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.IsSystemAdmin || stored.TenantID != 0 || stored.CanAccessAllTenants {
		t.Fatalf("stored platform identity = %+v", stored)
	}
}

func TestPromoteSystemAdminRejectsTenantlessMembership(t *testing.T) {
	db := activationTestDB(t)
	tenant := &types.Tenant{Name: "Acme", Status: types.TenantStatusActive}
	if err := db.Create(tenant).Error; err != nil {
		t.Fatal(err)
	}
	user := &types.User{
		ID: "tenantless-member", Username: "tenantless-member", Email: "tenantless-member@example.invalid",
		PasswordHash: "unused", IsActive: true,
	}
	if err := db.Omit("TenantID").Create(user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&types.TenantMember{
		UserID: user.ID, TenantID: tenant.ID, Role: types.TenantRoleViewer,
		Status: types.TenantMemberStatusActive,
	}).Error; err != nil {
		t.Fatal(err)
	}
	user.IsSystemAdmin = true
	_, err := (&userRepository{db: db}).PromoteSystemAdmin(context.Background(), user.ID)
	if !errors.Is(err, ErrSystemAdminEnterpriseIdentity) {
		t.Fatalf("promotion error = %v, want membership conflict", err)
	}
}

func TestUpdateUserCannotRevokePlatformAuthorityOrOverwriteBinding(t *testing.T) {
	db := activationTestDB(t)
	user := &types.User{
		ID: "operator", Username: "operator", Email: "operator@example.invalid",
		PasswordHash: "unused", IsActive: false, IsSystemAdmin: true,
	}
	if err := db.Omit("TenantID").Create(user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&types.User{}).Where("id = ?", user.ID).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	stale := *user
	stale.IsSystemAdmin = false
	stale.IsActive = true
	stale.CanAccessAllTenants = true
	stale.TenantID = 99
	stale.PasswordHash = "stale-password"
	if err := (&userRepository{db: db}).UpdateUser(context.Background(), &stale); err != nil {
		t.Fatal(err)
	}
	var stored types.User
	if err := db.First(&stored, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.IsSystemAdmin || stored.IsActive || stored.CanAccessAllTenants || stored.TenantID != 0 || stored.PasswordHash != "unused" {
		t.Fatalf("general update changed authority/binding: %+v", stored)
	}
}

func TestUpdateUserCannotOverwriteExplicitCredentialUpdate(t *testing.T) {
	db := activationTestDB(t)
	user := &types.User{
		ID: "employee", Username: "employee", Email: "employee@example.invalid",
		PasswordHash: "old-hash", IsActive: true,
	}
	if err := db.Omit("TenantID").Create(user).Error; err != nil {
		t.Fatal(err)
	}
	stale := *user
	repo := &userRepository{db: db}
	if err := repo.UpdateCredential(context.Background(), user.ID, "new-hash", nil); err != nil {
		t.Fatal(err)
	}
	stale.Username = "renamed"
	if err := repo.UpdateUser(context.Background(), &stale); err != nil {
		t.Fatal(err)
	}
	var stored types.User
	if err := db.First(&stored, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PasswordHash != "new-hash" || stored.Username != "renamed" {
		t.Fatalf("stored user = %+v", stored)
	}
}

func TestRevokeSystemAdminPreservesNullTenantBinding(t *testing.T) {
	db := activationTestDB(t)
	for _, id := range []string{"actor", "target"} {
		if err := db.Omit("TenantID").Create(&types.User{
			ID: id, Username: id, Email: id + "@example.invalid", PasswordHash: "unused",
			IsActive: true, IsSystemAdmin: true,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	revoked, err := (&userRepository{db: db}).RevokeSystemAdmin(context.Background(), "target", "actor")
	if err != nil {
		t.Fatal(err)
	}
	if revoked.IsSystemAdmin || revoked.CanAccessAllTenants || revoked.TenantID != 0 {
		t.Fatalf("revoked projection = %+v", revoked)
	}
	var tenantID sql.NullInt64
	if err := db.Raw("SELECT tenant_id FROM users WHERE id = ?", "target").Scan(&tenantID).Error; err != nil {
		t.Fatal(err)
	}
	if tenantID.Valid {
		t.Fatalf("raw tenant_id = %d, want NULL", tenantID.Int64)
	}
}

func TestCreateUserRejectsMixedPlatformIdentity(t *testing.T) {
	db := activationTestDB(t)
	err := (&userRepository{db: db}).CreateUser(context.Background(), &types.User{
		ID: "mixed", Username: "mixed", Email: "mixed@example.invalid", PasswordHash: "unused",
		TenantID: 7, IsActive: true, IsSystemAdmin: true,
	})
	if !errors.Is(err, ErrSystemAdminEnterpriseIdentity) {
		t.Fatalf("create mixed platform identity error = %v", err)
	}
}
