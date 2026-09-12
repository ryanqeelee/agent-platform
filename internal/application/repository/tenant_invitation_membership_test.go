package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestInvitationAcceptanceRejectsExistingSuspendedMembershipAtomically(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE tenants (
			id integer PRIMARY KEY, status varchar(32) NOT NULL DEFAULT 'active', deleted_at datetime
		)`,
		`CREATE TABLE users (
			id varchar(36) PRIMARY KEY, tenant_id integer,
			is_active boolean NOT NULL DEFAULT true,
			is_system_admin boolean NOT NULL DEFAULT false,
			can_access_all_tenants boolean NOT NULL DEFAULT false,
			deleted_at datetime
		)`,
		`CREATE TABLE tenant_members (
			id integer PRIMARY KEY AUTOINCREMENT, user_id varchar(36) NOT NULL,
			tenant_id integer NOT NULL, role varchar(20) NOT NULL,
			status varchar(20) NOT NULL, invited_by varchar(36), joined_at datetime,
			created_at datetime, updated_at datetime, deleted_at datetime
		)`,
		`CREATE TABLE tenant_invitations (
			id integer PRIMARY KEY AUTOINCREMENT, tenant_id integer NOT NULL,
			invitee_user_id varchar(36) NOT NULL DEFAULT '', token varchar(64) NOT NULL DEFAULT '',
			invited_by varchar(36), role varchar(20) NOT NULL,
			status varchar(20) NOT NULL DEFAULT 'pending', message varchar(500),
			expires_at datetime, responded_at datetime, created_at datetime,
			updated_at datetime, deleted_at datetime, accepted_count integer NOT NULL DEFAULT 0
		)`,
		`INSERT INTO tenants(id, status) VALUES (42, 'active')`,
		`INSERT INTO users(id, tenant_id) VALUES ('suspended-user', 42)`,
		`INSERT INTO tenant_members(user_id, tenant_id, role, status)
			VALUES ('suspended-user', 42, 'viewer', 'suspended')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}

	expiresAt := time.Now().Add(time.Hour)
	direct := &types.TenantInvitation{
		TenantID: 42, InviteeUserID: "suspended-user", Role: types.TenantRoleViewer,
		Status: types.TenantInvitationStatusPending, ExpiresAt: expiresAt,
	}
	share := &types.TenantInvitation{
		TenantID: 42, Token: "share-token", Role: types.TenantRoleViewer,
		Status: types.TenantInvitationStatusPending, ExpiresAt: expiresAt,
	}
	if err := db.Create(direct).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(share).Error; err != nil {
		t.Fatal(err)
	}

	repo := &tenantInvitationRepository{db: db}
	ctx := context.Background()
	if _, err := repo.AcceptInvitation(ctx, direct.ID, "suspended-user", time.Now()); !errors.Is(err, ErrInvitationMemberExists) {
		t.Fatalf("direct acceptance error = %v, want ErrInvitationMemberExists", err)
	}
	if _, err := repo.AcceptShareLink(ctx, share.ID, "suspended-user", time.Now()); !errors.Is(err, ErrInvitationMemberExists) {
		t.Fatalf("share acceptance error = %v, want ErrInvitationMemberExists", err)
	}

	for _, invitationID := range []uint64{direct.ID, share.ID} {
		var invitation types.TenantInvitation
		if err := db.First(&invitation, invitationID).Error; err != nil {
			t.Fatal(err)
		}
		if invitation.Status != types.TenantInvitationStatusPending ||
			invitation.AcceptedCount != 0 || invitation.RespondedAt != nil {
			t.Fatalf("rejected invitation %d was mutated: %+v", invitationID, invitation)
		}
	}

	var member types.TenantMember
	if err := db.Where("user_id = ? AND tenant_id = ?", "suspended-user", 42).
		First(&member).Error; err != nil {
		t.Fatal(err)
	}
	if member.Status != types.TenantMemberStatusSuspended || member.Role != types.TenantRoleViewer {
		t.Fatalf("suspended membership was altered: %+v", member)
	}
}

func TestInvitationAcceptanceRejectsPlatformIdentityAtomically(t *testing.T) {
	db := activationTestDB(t)
	if err := db.AutoMigrate(&types.TenantInvitation{}); err != nil {
		t.Fatal(err)
	}
	tenant := &types.Tenant{Name: "Acme", Status: types.TenantStatusActive}
	if err := db.Create(tenant).Error; err != nil {
		t.Fatal(err)
	}
	platform := &types.User{
		ID: "platform-admin", Username: "platform-admin", Email: "platform@example.invalid",
		PasswordHash: "unused", IsActive: true, IsSystemAdmin: true,
	}
	if err := db.Omit("TenantID").Create(platform).Error; err != nil {
		t.Fatal(err)
	}
	invitation := &types.TenantInvitation{
		TenantID: tenant.ID, InviteeUserID: platform.ID, Role: types.TenantRoleViewer,
		Status: types.TenantInvitationStatusPending, ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := db.Create(invitation).Error; err != nil {
		t.Fatal(err)
	}
	_, err := (&tenantInvitationRepository{db: db}).AcceptInvitation(
		context.Background(), invitation.ID, platform.ID, time.Now())
	if !errors.Is(err, ErrMemberActionForbidden) {
		t.Fatalf("accept platform identity error = %v, want forbidden", err)
	}
	var stored types.TenantInvitation
	if err := db.First(&stored, invitation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != types.TenantInvitationStatusPending || stored.AcceptedCount != 0 {
		t.Fatalf("rejected invitation mutated: %+v", stored)
	}
	var memberships int64
	if err := db.Model(&types.TenantMember{}).Where("user_id = ?", platform.ID).Count(&memberships).Error; err != nil {
		t.Fatal(err)
	}
	if memberships != 0 {
		t.Fatalf("platform identity gained %d memberships", memberships)
	}
}

func TestInvitationAcceptanceRequiresActiveEnterpriseAtomically(t *testing.T) {
	for _, status := range []string{
		types.TenantStatusSuspended,
		types.TenantStatusProvisioning,
		types.TenantStatusActivationAbandoned,
		types.TenantStatusActive,
	} {
		t.Run(status, func(t *testing.T) {
			db := activationTestDB(t)
			if err := db.AutoMigrate(&types.TenantInvitation{}); err != nil {
				t.Fatal(err)
			}
			tenant := &types.Tenant{Name: "Acme", Status: status}
			if err := db.Create(tenant).Error; err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"direct-user", "share-user"} {
				user := &types.User{
					ID: id, Username: id, Email: id + "@example.invalid",
					PasswordHash: "unused", IsActive: true,
				}
				if err := db.Omit("TenantID").Create(user).Error; err != nil {
					t.Fatal(err)
				}
			}
			expiresAt := time.Now().Add(time.Hour)
			direct := &types.TenantInvitation{
				TenantID: tenant.ID, InviteeUserID: "direct-user", Role: types.TenantRoleViewer,
				Status: types.TenantInvitationStatusPending, ExpiresAt: expiresAt,
			}
			share := &types.TenantInvitation{
				TenantID: tenant.ID, Token: "share-token", Role: types.TenantRoleViewer,
				Status: types.TenantInvitationStatusPending, ExpiresAt: expiresAt,
			}
			if err := db.Create(direct).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(share).Error; err != nil {
				t.Fatal(err)
			}

			repo := &tenantInvitationRepository{db: db}
			directMember, directErr := repo.AcceptInvitation(context.Background(), direct.ID, "direct-user", time.Now())
			shareMember, shareErr := repo.AcceptShareLink(context.Background(), share.ID, "share-user", time.Now())
			if status == types.TenantStatusActive {
				if directErr != nil || shareErr != nil || directMember == nil || shareMember == nil {
					t.Fatalf("active acceptance: direct=(%+v,%v) share=(%+v,%v)", directMember, directErr, shareMember, shareErr)
				}
				return
			}
			if !errors.Is(directErr, ErrEnterpriseNotActive) || !errors.Is(shareErr, ErrEnterpriseNotActive) {
				t.Fatalf("inactive acceptance errors: direct=%v share=%v", directErr, shareErr)
			}
			if directMember != nil || shareMember != nil {
				t.Fatalf("inactive acceptance returned membership: direct=%+v share=%+v", directMember, shareMember)
			}
			var memberships int64
			if err := db.Model(&types.TenantMember{}).Where("tenant_id = ?", tenant.ID).Count(&memberships).Error; err != nil {
				t.Fatal(err)
			}
			if memberships != 0 {
				t.Fatalf("inactive enterprise gained %d memberships", memberships)
			}
			for _, id := range []string{"direct-user", "share-user"} {
				var user types.User
				if err := db.First(&user, "id = ?", id).Error; err != nil {
					t.Fatal(err)
				}
				if user.TenantID != 0 {
					t.Fatalf("user %s bound to inactive tenant %d", id, user.TenantID)
				}
			}
			for _, invitationID := range []uint64{direct.ID, share.ID} {
				var invitation types.TenantInvitation
				if err := db.First(&invitation, invitationID).Error; err != nil {
					t.Fatal(err)
				}
				if invitation.Status != types.TenantInvitationStatusPending || invitation.AcceptedCount != 0 || invitation.RespondedAt != nil {
					t.Fatalf("rejected invitation %d mutated: %+v", invitationID, invitation)
				}
			}
		})
	}
}

func TestExistingMemberInvitationAcceptanceRequiresActiveEnterpriseAtomically(t *testing.T) {
	for _, status := range []string{
		types.TenantStatusSuspended,
		types.TenantStatusProvisioning,
		types.TenantStatusActivationAbandoned,
		types.TenantStatusActive,
	} {
		t.Run(status, func(t *testing.T) {
			db := activationTestDB(t)
			if err := db.AutoMigrate(&types.TenantInvitation{}); err != nil {
				t.Fatal(err)
			}
			tenant := &types.Tenant{Name: "Acme", Status: status}
			if err := db.Create(tenant).Error; err != nil {
				t.Fatal(err)
			}
			user := &types.User{
				ID: "existing-user", Username: "existing-user", Email: "existing@example.invalid",
				PasswordHash: "unused", TenantID: tenant.ID, IsActive: true,
			}
			if err := db.Create(user).Error; err != nil {
				t.Fatal(err)
			}
			member := &types.TenantMember{
				UserID: user.ID, TenantID: tenant.ID, Role: types.TenantRoleViewer,
				Status: types.TenantMemberStatusActive, JoinedAt: time.Now(),
			}
			if err := db.Create(member).Error; err != nil {
				t.Fatal(err)
			}
			expiresAt := time.Now().Add(time.Hour)
			direct := &types.TenantInvitation{
				TenantID: tenant.ID, InviteeUserID: user.ID, Role: types.TenantRoleViewer,
				Status: types.TenantInvitationStatusPending, ExpiresAt: expiresAt,
			}
			share := &types.TenantInvitation{
				TenantID: tenant.ID, Token: "existing-share-token", Role: types.TenantRoleViewer,
				Status: types.TenantInvitationStatusPending, ExpiresAt: expiresAt,
			}
			if err := db.Create(direct).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(share).Error; err != nil {
				t.Fatal(err)
			}

			repo := &tenantInvitationRepository{db: db}
			directMember, directErr := repo.AcceptInvitation(context.Background(), direct.ID, user.ID, time.Now())
			shareMember, shareErr := repo.AcceptShareLink(context.Background(), share.ID, user.ID, time.Now())
			if status == types.TenantStatusActive {
				if directErr != nil || shareErr != nil || directMember == nil || shareMember == nil {
					t.Fatalf("active existing-member acceptance: direct=(%+v,%v) share=(%+v,%v)", directMember, directErr, shareMember, shareErr)
				}
				replayedShareMember, replayedShareErr := repo.AcceptShareLink(context.Background(), share.ID, user.ID, time.Now())
				if replayedShareErr != nil || replayedShareMember == nil {
					t.Fatalf("active existing-member share replay = (%+v,%v), want success", replayedShareMember, replayedShareErr)
				}
				var count int64
				if err := db.Model(&types.TenantMember{}).
					Where("user_id = ? AND tenant_id = ?", user.ID, tenant.ID).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 1 {
					t.Fatalf("active existing-member acceptance created %d memberships, want 1", count)
				}
				var storedDirect, storedShare types.TenantInvitation
				if err := db.First(&storedDirect, direct.ID).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.First(&storedShare, share.ID).Error; err != nil {
					t.Fatal(err)
				}
				if storedDirect.Status != types.TenantInvitationStatusAccepted || storedDirect.AcceptedCount != 1 {
					t.Fatalf("active direct invitation = %+v, want accepted once", storedDirect)
				}
				if storedShare.Status != types.TenantInvitationStatusPending || storedShare.AcceptedCount != 0 {
					t.Fatalf("idempotent existing-member share invitation mutated: %+v", storedShare)
				}
				return
			}
			if !errors.Is(directErr, ErrEnterpriseNotActive) || !errors.Is(shareErr, ErrEnterpriseNotActive) {
				t.Fatalf("inactive existing-member errors: direct=%v share=%v", directErr, shareErr)
			}
			for _, invitationID := range []uint64{direct.ID, share.ID} {
				var stored types.TenantInvitation
				if err := db.First(&stored, invitationID).Error; err != nil {
					t.Fatal(err)
				}
				if stored.Status != types.TenantInvitationStatusPending || stored.AcceptedCount != 0 || stored.RespondedAt != nil {
					t.Fatalf("inactive existing-member invitation %d mutated: %+v", invitationID, stored)
				}
			}
			var storedMember types.TenantMember
			if err := db.Where("user_id = ? AND tenant_id = ?", user.ID, tenant.ID).First(&storedMember).Error; err != nil {
				t.Fatal(err)
			}
			if storedMember.Status != types.TenantMemberStatusActive || storedMember.Role != types.TenantRoleViewer {
				t.Fatalf("inactive existing membership mutated: %+v", storedMember)
			}
			var storedUser types.User
			if err := db.First(&storedUser, "id = ?", user.ID).Error; err != nil {
				t.Fatal(err)
			}
			if storedUser.TenantID != tenant.ID {
				t.Fatalf("inactive existing user binding = %d, want %d", storedUser.TenantID, tenant.ID)
			}
		})
	}
}
