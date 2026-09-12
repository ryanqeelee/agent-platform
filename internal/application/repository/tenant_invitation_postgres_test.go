package repository

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestExistingMemberInvitationAcceptanceSerializedWithSuspensionPostgres(t *testing.T) {
	dsn := os.Getenv("WEKNORA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("WEKNORA_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := "weknora_invitation_" + uuid.NewString()[:8]
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error })

	scopedDSN := dsn + " search_path=" + schema
	if strings.Contains(dsn, "://") {
		parsed, parseErr := url.Parse(dsn)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		query := parsed.Query()
		query.Set("search_path", schema)
		parsed.RawQuery = query.Encode()
		scopedDSN = parsed.String()
	}
	db, err := gorm.Open(postgres.Open(scopedDSN), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE tenants (
			id bigint PRIMARY KEY, status varchar(32) NOT NULL DEFAULT 'active',
			seats_total integer, deleted_at timestamptz
		)`,
		`CREATE TABLE users (
			id varchar(36) PRIMARY KEY, tenant_id bigint,
			is_active boolean NOT NULL DEFAULT true,
			is_system_admin boolean NOT NULL DEFAULT false,
			can_access_all_tenants boolean NOT NULL DEFAULT false,
			deleted_at timestamptz
		)`,
		`CREATE TABLE tenant_members (
			id bigserial PRIMARY KEY, user_id varchar(36) NOT NULL, tenant_id bigint NOT NULL,
			role varchar(20) NOT NULL, status varchar(20) NOT NULL,
			invited_by varchar(36), joined_at timestamptz,
			created_at timestamptz, updated_at timestamptz, deleted_at timestamptz
		)`,
		`CREATE TABLE tenant_invitations (
			id bigserial PRIMARY KEY, tenant_id bigint NOT NULL,
			invitee_user_id varchar(36) NOT NULL DEFAULT '', token varchar(64) NOT NULL DEFAULT '',
			invited_by varchar(36), role varchar(20) NOT NULL,
			status varchar(20) NOT NULL DEFAULT 'pending', expires_at timestamptz,
			responded_at timestamptz, accepted_count integer NOT NULL DEFAULT 0,
			updated_at timestamptz, deleted_at timestamptz
		)`,
		`INSERT INTO tenants(id, status) VALUES (1, 'active')`,
		`INSERT INTO users(id, tenant_id) VALUES ('existing-user', 1)`,
		`INSERT INTO tenant_members(user_id, tenant_id, role, status, joined_at)
			VALUES ('existing-user', 1, 'viewer', 'active', CURRENT_TIMESTAMP)`,
		`INSERT INTO tenant_invitations(id, tenant_id, invitee_user_id, role, status, expires_at)
			VALUES (1, 1, 'existing-user', 'viewer', 'pending', CURRENT_TIMESTAMP + INTERVAL '1 hour'),
			       (2, 1, 'existing-user', 'viewer', 'pending', CURRENT_TIMESTAMP + INTERVAL '1 hour')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}

	// Hold the tenant row while suspension is in flight. Acceptance first locks
	// the invitation, then waits for this row and must observe the committed
	// suspended status before inspecting the existing membership.
	suspension := db.Begin()
	if suspension.Error != nil {
		t.Fatal(suspension.Error)
	}
	if err := suspension.Exec(`UPDATE tenants SET status = 'suspended' WHERE id = 1`).Error; err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, acceptErr := (&tenantInvitationRepository{db: db}).AcceptInvitation(
			context.Background(), 1, "existing-user", time.Now())
		result <- acceptErr
	}()
	select {
	case acceptErr := <-result:
		_ = suspension.Rollback().Error
		t.Fatalf("acceptance bypassed in-flight suspension: %v", acceptErr)
	case <-time.After(250 * time.Millisecond):
	}
	if err := suspension.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case acceptErr := <-result:
		if !errors.Is(acceptErr, ErrEnterpriseNotActive) {
			t.Fatalf("racing acceptance error = %v, want ErrEnterpriseNotActive", acceptErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("acceptance did not finish after suspension committed")
	}

	if _, err := (&tenantInvitationRepository{db: db}).AcceptInvitation(
		context.Background(), 2, "existing-user", time.Now()); !errors.Is(err, ErrEnterpriseNotActive) {
		t.Fatalf("post-suspension acceptance error = %v, want ErrEnterpriseNotActive", err)
	}
	for _, invitationID := range []uint64{1, 2} {
		var status string
		var acceptedCount int
		var respondedAt *time.Time
		if err := db.Raw(`SELECT status, accepted_count, responded_at FROM tenant_invitations WHERE id = ?`, invitationID).
			Row().Scan(&status, &acceptedCount, &respondedAt); err != nil {
			t.Fatal(err)
		}
		if status != string(types.TenantInvitationStatusPending) || acceptedCount != 0 || respondedAt != nil {
			t.Fatalf("invitation %d mutated: status=%s accepted_count=%d responded_at=%v", invitationID, status, acceptedCount, respondedAt)
		}
	}
	var membershipCount int64
	if err := db.Raw(`SELECT count(*) FROM tenant_members WHERE user_id = 'existing-user' AND tenant_id = 1 AND status = 'active'`).
		Scan(&membershipCount).Error; err != nil {
		t.Fatal(err)
	}
	if membershipCount != 1 {
		t.Fatalf("active membership count = %d, want 1", membershipCount)
	}
	var tenantID uint64
	if err := db.Raw(`SELECT tenant_id FROM users WHERE id = 'existing-user'`).Scan(&tenantID).Error; err != nil {
		t.Fatal(err)
	}
	if tenantID != 1 {
		t.Fatalf("user binding = %d, want 1", tenantID)
	}
}
