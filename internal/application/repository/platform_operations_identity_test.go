package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func platformIdentityTestDB(t *testing.T) (*platformOperationsIdentityRepository, *types.Tenant) {
	t.Helper()
	db := activationTestDB(t)
	require.NoError(t, db.AutoMigrate(&types.PlatformInitialAdministratorReceipt{}, &types.AuthToken{}))
	require.NoError(t, db.Omit("TenantID").Create(&types.User{
		ID: "system-admin", Username: "system-admin", Email: "system-admin@example.invalid",
		PasswordHash: "unused", IsActive: true, IsSystemAdmin: true,
	}).Error)
	tenant := &types.Tenant{Name: "Acme", Status: types.TenantStatusActive}
	require.NoError(t, db.Create(tenant).Error)
	return &platformOperationsIdentityRepository{db: db}, tenant
}

func TestPlatformInitialAdministratorReceiptReplaysOnlyItsCreatedIdentity(t *testing.T) {
	repo, _ := platformIdentityTestDB(t)
	password := "Password123!"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)
	first := &types.User{
		ID: "initial-admin-1", Username: "owner", Email: "owner@example.invalid",
		PasswordHash: string(hash), IsActive: true,
	}
	receipt, created, replayed, err := repo.CreateInitialAdministrator(
		context.Background(), "system-admin", "command-1", "digest-1", password, first)
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, "initial-admin-1", created.ID)
	require.Equal(t, "created", receipt.Status)

	secondHash, err := bcrypt.GenerateFromPassword([]byte("Replacement456!"), bcrypt.DefaultCost)
	require.NoError(t, err)
	replayCandidate := &types.User{
		ID: "must-not-be-created", Username: first.Username, Email: first.Email,
		PasswordHash: string(secondHash), IsActive: true,
	}
	receipt, replayedUser, replayed, err := repo.CreateInitialAdministrator(
		context.Background(), "system-admin", "command-1", "digest-1", password, replayCandidate)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, "initial-admin-1", receipt.UserID)
	require.Equal(t, "initial-admin-1", replayedUser.ID)
	require.Equal(t, string(hash), replayedUser.PasswordHash, "a replay must not reset the existing password")

	_, _, _, err = repo.CreateInitialAdministrator(
		context.Background(), "system-admin", "command-1", "different-digest", password,
		&types.User{ID: "other", Username: "other", Email: "other@example.invalid", PasswordHash: string(hash), IsActive: true})
	require.ErrorIs(t, err, ErrPlatformOperationConflict)
	_, _, _, err = repo.CreateInitialAdministrator(
		context.Background(), "system-admin", "command-1", "digest-1", "Different789!", replayCandidate)
	require.ErrorIs(t, err, ErrPlatformOperationConflict)

	storedReceipt, storedUser, err := repo.GetInitialAdministrator(context.Background(), "system-admin", "command-1")
	require.NoError(t, err)
	require.Equal(t, "initial-admin-1", storedReceipt.UserID)
	require.Equal(t, "initial-admin-1", storedUser.ID)
}

func TestPlatformInitialAdministratorDoesNotAdoptPreexistingIdentity(t *testing.T) {
	repo, _ := platformIdentityTestDB(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("Password123!"), bcrypt.DefaultCost)
	require.NoError(t, err)
	require.NoError(t, repo.db.Omit("TenantID").Create(&types.User{
		ID: "preexisting", Username: "owner", Email: "owner@example.invalid",
		PasswordHash: string(hash), IsActive: true,
	}).Error)

	_, _, _, err = repo.CreateInitialAdministrator(
		context.Background(), "system-admin", "new-command", "digest", "Password123!",
		&types.User{ID: "new-user", Username: "owner", Email: "owner@example.invalid", PasswordHash: string(hash), IsActive: true})
	require.ErrorIs(t, err, ErrPlatformOperationConflict)
	_, _, err = repo.GetInitialAdministrator(context.Background(), "system-admin", "new-command")
	require.ErrorIs(t, err, ErrPlatformOperationNotFound)
}

func TestPlatformPasswordResetRechecksAuthorityBindingAndRevokesSessions(t *testing.T) {
	repo, tenant := platformIdentityTestDB(t)
	oldHash, err := bcrypt.GenerateFromPassword([]byte("OldPassword123!"), bcrypt.DefaultCost)
	require.NoError(t, err)
	target := &types.User{
		ID: "employee", Username: "employee", Email: "employee@example.invalid",
		PasswordHash: string(oldHash), TenantID: tenant.ID, IsActive: true,
	}
	require.NoError(t, repo.db.Create(target).Error)
	require.NoError(t, repo.db.Create(&types.TenantMember{
		UserID: target.ID, TenantID: tenant.ID, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive,
	}).Error)
	require.NoError(t, repo.db.Create(&types.AuthToken{
		ID: "access-token", UserID: target.ID, Token: "opaque", TokenType: "access_token",
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error)
	newHash, err := bcrypt.GenerateFromPassword([]byte("NewPassword456!"), bcrypt.DefaultCost)
	require.NoError(t, err)
	require.NoError(t, repo.ResetEnterpriseMemberPassword(
		context.Background(), "system-admin", tenant.ID, target.ID, string(newHash)))

	var stored types.User
	require.NoError(t, repo.db.First(&stored, "id = ?", target.ID).Error)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("NewPassword456!")))
	var token types.AuthToken
	require.NoError(t, repo.db.First(&token, "id = ?", "access-token").Error)
	require.True(t, token.IsRevoked)

	unchangedHash := stored.PasswordHash
	require.NoError(t, repo.db.Model(&types.User{}).Where("id = ?", "system-admin").Update("is_system_admin", false).Error)
	anotherHash, err := bcrypt.GenerateFromPassword([]byte("AnotherPassword789!"), bcrypt.DefaultCost)
	require.NoError(t, err)
	err = repo.ResetEnterpriseMemberPassword(context.Background(), "system-admin", tenant.ID, target.ID, string(anotherHash))
	require.ErrorIs(t, err, ErrMemberActionForbidden)
	require.NoError(t, repo.db.First(&stored, "id = ?", target.ID).Error)
	require.Equal(t, unchangedHash, stored.PasswordHash)

	require.NoError(t, repo.db.Model(&types.User{}).Where("id = ?", "system-admin").Update("is_system_admin", true).Error)
	otherTenant := &types.Tenant{Name: "Other", Status: types.TenantStatusActive}
	require.NoError(t, repo.db.Create(otherTenant).Error)
	require.NoError(t, repo.db.Model(&types.User{}).Where("id = ?", target.ID).Update("tenant_id", otherTenant.ID).Error)
	err = repo.ResetEnterpriseMemberPassword(context.Background(), "system-admin", tenant.ID, target.ID, string(anotherHash))
	require.True(t, errors.Is(err, ErrPlatformOperationTargetForbidden), "err=%v", err)
}

func TestPlatformPasswordResetRejectsIncompleteEnterpriseWithoutMutation(t *testing.T) {
	for _, status := range []string{types.TenantStatusSuspended, types.TenantStatusProvisioning, types.TenantStatusActivationAbandoned} {
		t.Run(status, func(t *testing.T) {
			repo, tenant := platformIdentityTestDB(t)
			require.NoError(t, repo.db.Model(&types.Tenant{}).Where("id = ?", tenant.ID).Update("status", status).Error)
			oldHash, err := bcrypt.GenerateFromPassword([]byte("OldPassword123!"), bcrypt.DefaultCost)
			require.NoError(t, err)
			target := &types.User{
				ID: "lifecycle-employee", Username: "lifecycle-employee", Email: "lifecycle-employee@example.invalid",
				PasswordHash: string(oldHash), TenantID: tenant.ID, IsActive: true,
			}
			require.NoError(t, repo.db.Create(target).Error)
			require.NoError(t, repo.db.Create(&types.TenantMember{
				UserID: target.ID, TenantID: tenant.ID, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive,
			}).Error)
			require.NoError(t, repo.db.Create(&types.AuthToken{
				ID: "lifecycle-token", UserID: target.ID, Token: "opaque", TokenType: "access_token",
				ExpiresAt: time.Now().Add(time.Hour),
			}).Error)
			newHash, err := bcrypt.GenerateFromPassword([]byte("NewPassword456!"), bcrypt.DefaultCost)
			require.NoError(t, err)

			err = repo.ResetEnterpriseMemberPassword(
				context.Background(), "system-admin", tenant.ID, target.ID, string(newHash))
			require.ErrorIs(t, err, ErrEnterpriseNotActive)
			var stored types.User
			require.NoError(t, repo.db.First(&stored, "id = ?", target.ID).Error)
			require.Equal(t, string(oldHash), stored.PasswordHash)
			var token types.AuthToken
			require.NoError(t, repo.db.First(&token, "id = ?", "lifecycle-token").Error)
			require.False(t, token.IsRevoked)
		})
	}
}

func TestPlatformPasswordResetRequiresCurrentActiveMembership(t *testing.T) {
	for _, membershipState := range []string{"suspended", "removed"} {
		t.Run(membershipState, func(t *testing.T) {
			repo, tenant := platformIdentityTestDB(t)
			oldHash, err := bcrypt.GenerateFromPassword([]byte("OldPassword123!"), bcrypt.DefaultCost)
			require.NoError(t, err)
			target := &types.User{
				ID: "membership-employee", Username: "membership-employee", Email: "membership-employee@example.invalid",
				PasswordHash: string(oldHash), TenantID: tenant.ID, IsActive: true,
			}
			require.NoError(t, repo.db.Create(target).Error)
			member := &types.TenantMember{
				UserID: target.ID, TenantID: tenant.ID, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive,
			}
			require.NoError(t, repo.db.Create(member).Error)
			if membershipState == "suspended" {
				require.NoError(t, repo.db.Model(member).Update("status", types.TenantMemberStatusSuspended).Error)
			} else {
				require.NoError(t, repo.db.Delete(member).Error)
			}
			require.NoError(t, repo.db.Create(&types.AuthToken{
				ID: "membership-token", UserID: target.ID, Token: "opaque", TokenType: "access_token",
				ExpiresAt: time.Now().Add(time.Hour),
			}).Error)
			newHash, err := bcrypt.GenerateFromPassword([]byte("NewPassword456!"), bcrypt.DefaultCost)
			require.NoError(t, err)

			err = repo.ResetEnterpriseMemberPassword(
				context.Background(), "system-admin", tenant.ID, target.ID, string(newHash))
			require.ErrorIs(t, err, ErrPlatformOperationTargetForbidden)
			var stored types.User
			require.NoError(t, repo.db.First(&stored, "id = ?", target.ID).Error)
			require.Equal(t, string(oldHash), stored.PasswordHash)
			var token types.AuthToken
			require.NoError(t, repo.db.First(&token, "id = ?", "membership-token").Error)
			require.False(t, token.IsRevoked)
		})
	}
}

func TestLegacyTenantUpdateDoesNotOverwriteOperationsFields(t *testing.T) {
	repo, tenant := platformIdentityTestDB(t)
	initialSeats := 10
	require.NoError(t, repo.db.Model(&types.Tenant{}).Where("id = ?", tenant.ID).Updates(map[string]any{
		"seats_total": initialSeats, "storage_quota": 1024,
	}).Error)
	var stale types.Tenant
	require.NoError(t, repo.db.First(&stale, tenant.ID).Error)

	reducedSeats := 5
	_, _, err := NewPlatformOperationsTenantRepository(repo.db).UpdateForPlatformOperations(
		context.Background(), "system-admin", tenant.ID, "Operations renamed", "Operations", types.TenantStatusSuspended, true, &reducedSeats, 2048)
	require.NoError(t, err)

	stale.Name = "stale configuration snapshot"
	stale.Description = "stale description"
	stale.Business = "updated configuration"
	require.NoError(t, NewTenantRepository(repo.db).UpdateTenant(context.Background(), &stale))
	var stored types.Tenant
	require.NoError(t, repo.db.First(&stored, tenant.ID).Error)
	require.Equal(t, "Operations renamed", stored.Name)
	require.Equal(t, "Operations", stored.Description)
	require.Equal(t, "updated configuration", stored.Business)
	require.Equal(t, types.TenantStatusSuspended, stored.Status)
	require.NotNil(t, stored.SeatsTotal)
	require.Equal(t, reducedSeats, *stored.SeatsTotal)
	require.Equal(t, int64(2048), stored.StorageQuota)

	_, _, err = NewPlatformOperationsTenantRepository(repo.db).UpdateForPlatformOperations(
		context.Background(), "system-admin", tenant.ID, "Operations final", "Operations final description",
		types.TenantStatusActive, false, &reducedSeats, 4096)
	require.NoError(t, err)
	profileDescription := "profile description only"
	require.NoError(t, NewTenantRepository(repo.db).UpdateTenantProfile(
		context.Background(), tenant.ID, nil, &profileDescription))
	require.NoError(t, repo.db.First(&stored, tenant.ID).Error)
	require.Equal(t, "Operations final", stored.Name, "an omitted profile field must not overwrite a concurrent operations patch")
	require.Equal(t, profileDescription, stored.Description)
	require.Equal(t, types.TenantStatusActive, stored.Status)
	require.Equal(t, int64(4096), stored.StorageQuota)
}
