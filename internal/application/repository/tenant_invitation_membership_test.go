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
		`CREATE TABLE users (
			id varchar(36) PRIMARY KEY, tenant_id integer, deleted_at datetime
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
