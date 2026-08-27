package repository

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestEnterpriseManagedTenantBindingConcurrent(t *testing.T) {
	dsn := os.Getenv("WEKNORA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("WEKNORA_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := "weknora_enterprise_" + uuid.NewString()[:8]
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
		`CREATE TABLE tenants (id bigint PRIMARY KEY, deleted_at timestamptz)`,
		`CREATE TABLE users (
            id varchar(36) PRIMARY KEY, tenant_id bigint,
			username varchar(255) UNIQUE, email varchar(255) UNIQUE,
            updated_at timestamptz, deleted_at timestamptz
        )`,
		`CREATE TABLE tenant_members (
            id bigserial PRIMARY KEY, user_id varchar(36) NOT NULL, tenant_id bigint NOT NULL,
            role varchar(20) NOT NULL, status varchar(20) NOT NULL, invited_by varchar(36),
            joined_at timestamptz, created_at timestamptz, updated_at timestamptz, deleted_at timestamptz
        )`,
		`CREATE UNIQUE INDEX uniq_user_tenant ON tenant_members(user_id, tenant_id) WHERE deleted_at IS NULL`,
		`CREATE TABLE tenant_invitations (
			id bigserial PRIMARY KEY, tenant_id bigint NOT NULL, invitee_user_id varchar(36) NOT NULL DEFAULT '',
			invited_by varchar(36), role varchar(20) NOT NULL, status varchar(20) NOT NULL DEFAULT 'pending',
			expires_at timestamptz, responded_at timestamptz, accepted_count integer NOT NULL DEFAULT 0,
			updated_at timestamptz, deleted_at timestamptz
		)`,
		`INSERT INTO tenants(id) VALUES (101), (202), (303), (404), (505)`,
		`INSERT INTO users(id, tenant_id) VALUES ('u1', 0)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}

	repo := &tenantMemberRepository{db: db}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, tenantID := range []uint64{101, 202} {
		wg.Add(1)
		go func(tenantID uint64) {
			defer wg.Done()
			<-start
			results <- repo.Create(context.Background(), &types.TenantMember{
				UserID: "u1", TenantID: tenantID, Role: types.TenantRoleViewer,
				Status: types.TenantMemberStatusActive, JoinedAt: time.Now(),
			})
		}(tenantID)
	}
	close(start)
	wg.Wait()
	close(results)

	var succeeded, rejected int
	for result := range results {
		switch {
		case result == nil:
			succeeded++
		case errors.Is(result, ErrUserBoundToAnotherEnterprise):
			rejected++
		default:
			t.Fatalf("unexpected create result: %v", result)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("succeeded=%d rejected=%d", succeeded, rejected)
	}

	var home uint64
	if err := db.Raw(`SELECT tenant_id FROM users WHERE id = 'u1'`).Scan(&home).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM tenant_members WHERE user_id = 'u1'`).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	var memberTenant uint64
	if err := db.Raw(`SELECT tenant_id FROM tenant_members WHERE user_id = 'u1'`).Scan(&memberTenant).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 || home != memberTenant {
		t.Fatalf("home=%d member=%d count=%d", home, memberTenant, count)
	}

	foreign := uint64(101)
	if foreign == home {
		foreign = 202
	}
	if err := db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status, joined_at) VALUES ('u1', ?, 'owner', 'active', CURRENT_TIMESTAMP)`, foreign).Error; err != nil {
		t.Fatal(err)
	}
	if member, err := repo.Get(context.Background(), "u1", foreign); err != nil || member != nil {
		t.Fatalf("historical foreign membership visible through Get: member=%v err=%v", member, err)
	}
	if members, err := repo.ListByTenant(context.Background(), foreign); err != nil || len(members) != 0 {
		t.Fatalf("historical foreign membership visible in roster: members=%v err=%v", members, err)
	}
	if any, err := repo.HasAnyMembers(context.Background(), foreign); err != nil || any {
		t.Fatalf("historical foreign membership counted as active: any=%v err=%v", any, err)
	}
	if owners, err := repo.CountActiveOwners(context.Background(), foreign); err != nil || owners != 0 {
		t.Fatalf("historical foreign owner affected owner count: owners=%d err=%v", owners, err)
	}
	if err := db.Exec(`INSERT INTO tenant_invitations(id, tenant_id, invitee_user_id, role, status, expires_at) VALUES (2, ?, 'u1', 'contributor', 'pending', CURRENT_TIMESTAMP + INTERVAL '1 hour')`, foreign).Error; err != nil {
		t.Fatal(err)
	}
	invRepo := &tenantInvitationRepository{db: db}
	if _, err := invRepo.AcceptInvitation(context.Background(), 2, "u1", time.Now()); !errors.Is(err, ErrUserBoundToAnotherEnterprise) {
		t.Fatalf("historical foreign membership bypassed enterprise binding: %v", err)
	}
	var foreignInvitationStatus string
	if err := db.Raw(`SELECT status FROM tenant_invitations WHERE id = 2`).Scan(&foreignInvitationStatus).Error; err != nil || foreignInvitationStatus != "pending" {
		t.Fatalf("foreign invitation status=%s err=%v", foreignInvitationStatus, err)
	}

	if err := db.Exec(`INSERT INTO users(id, tenant_id) VALUES ('u2', 0)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tenant_invitations(id, tenant_id, invitee_user_id, role, status, expires_at) VALUES (1, 303, 'u2', 'viewer', 'pending', CURRENT_TIMESTAMP + INTERVAL '1 hour')`).Error; err != nil {
		t.Fatal(err)
	}
	start = make(chan struct{})
	results = make(chan error, 2)
	wg = sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, acceptErr := invRepo.AcceptInvitation(context.Background(), 1, "u2", time.Now())
		results <- acceptErr
	}()
	go func() {
		defer wg.Done()
		<-start
		results <- invRepo.MarkStatusIfPending(context.Background(), types.MemberActorAuthority{ServicePrincipal: true}, 1, types.TenantInvitationStatusRevoked, time.Now())
	}()
	close(start)
	wg.Wait()
	close(results)
	var raceSuccess int
	for raceErr := range results {
		if raceErr == nil {
			raceSuccess++
			continue
		}
		if !errors.Is(raceErr, ErrInvitationNotPending) && !errors.Is(raceErr, gorm.ErrRecordNotFound) {
			t.Fatalf("unexpected accept/revoke result: %v", raceErr)
		}
	}
	if raceSuccess != 1 {
		t.Fatalf("accept/revoke successes=%d, want exactly one", raceSuccess)
	}
	var invitationStatus string
	if err := db.Raw(`SELECT status FROM tenant_invitations WHERE id = 1`).Scan(&invitationStatus).Error; err != nil {
		t.Fatal(err)
	}
	var acceptedMemberships int64
	if err := db.Raw(`SELECT count(*) FROM tenant_members WHERE user_id = 'u2' AND tenant_id = 303`).Scan(&acceptedMemberships).Error; err != nil {
		t.Fatal(err)
	}
	if (invitationStatus == "accepted") != (acceptedMemberships == 1) {
		t.Fatalf("status=%s memberships=%d", invitationStatus, acceptedMemberships)
	}

	if err := db.Exec(`INSERT INTO users(id, tenant_id) VALUES ('u3', 0)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tenant_invitations(id, tenant_id, invitee_user_id, role, status, expires_at) VALUES (3, 404, '', 'viewer', 'pending', CURRENT_TIMESTAMP + INTERVAL '1 hour')`).Error; err != nil {
		t.Fatal(err)
	}
	start = make(chan struct{})
	results = make(chan error, 2)
	wg = sync.WaitGroup{}
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, acceptErr := invRepo.AcceptShareLink(context.Background(), 3, "u3", time.Now())
		results <- acceptErr
	}()
	go func() {
		defer wg.Done()
		<-start
		results <- repo.Create(context.Background(), &types.TenantMember{
			UserID: "u3", TenantID: 505, Role: types.TenantRoleViewer,
			Status: types.TenantMemberStatusActive, JoinedAt: time.Now(),
		})
	}()
	close(start)
	wg.Wait()
	close(results)
	var firstBindingSuccess int
	for bindErr := range results {
		if bindErr == nil {
			firstBindingSuccess++
			continue
		}
		if !errors.Is(bindErr, ErrUserBoundToAnotherEnterprise) {
			t.Fatalf("unexpected first-binding result: %v", bindErr)
		}
	}
	if firstBindingSuccess != 1 {
		t.Fatalf("first-binding successes=%d, want exactly one", firstBindingSuccess)
	}
	var u3Home uint64
	if err := db.Raw(`SELECT tenant_id FROM users WHERE id = 'u3'`).Scan(&u3Home).Error; err != nil {
		t.Fatal(err)
	}
	var u3Memberships int64
	if err := db.Raw(`SELECT count(*) FROM tenant_members WHERE user_id = 'u3' AND tenant_id = ?`, u3Home).Scan(&u3Memberships).Error; err != nil {
		t.Fatal(err)
	}
	if u3Memberships != 1 {
		t.Fatalf("home=%d matching memberships=%d, want 1", u3Home, u3Memberships)
	}

	if err := db.Exec(`INSERT INTO users(id, tenant_id, username, email) VALUES ('u4', NULL, 'retry-user', 'retry@example.invalid')`).Error; err != nil {
		t.Fatal(err)
	}
	userRepo := &userRepository{db: db}
	if err := userRepo.DeleteTenantlessUser(context.Background(), "u4"); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO users(id, tenant_id, username, email) VALUES ('u5', NULL, 'retry-user', 'retry@example.invalid')`).Error; err != nil {
		t.Fatalf("same-email retry remained blocked after incomplete-account cleanup: %v", err)
	}
}

func TestTenantMemberRepository_TransferOwnershipPostgres(t *testing.T) {
	dsn := os.Getenv("WEKNORA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("WEKNORA_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := "weknora_transfer_" + uuid.NewString()[:8]
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error
	})
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
		`CREATE TABLE tenants (id bigint PRIMARY KEY, deleted_at timestamptz)`,
		`CREATE TABLE users (id varchar(36) PRIMARY KEY, tenant_id bigint, deleted_at timestamptz)`,
		`CREATE TABLE tenant_members (
            id bigserial PRIMARY KEY, user_id varchar(36) NOT NULL, tenant_id bigint NOT NULL,
            role varchar(20) NOT NULL, status varchar(20) NOT NULL, updated_at timestamptz, deleted_at timestamptz
        )`,
		`CREATE UNIQUE INDEX one_active_owner ON tenant_members(tenant_id)
            WHERE role = 'owner' AND status = 'active' AND deleted_at IS NULL`,
		`INSERT INTO tenants(id) VALUES (1)`,
		`INSERT INTO users(id, tenant_id) VALUES ('owner', 1), ('admin-a', 1), ('admin-b', 1), ('employee', 1)`,
		`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES
            ('owner', 1, 'owner', 'active'), ('admin-a', 1, 'admin', 'active'),
            ('admin-b', 1, 'admin', 'active'), ('employee', 1, 'viewer', 'active')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := &tenantMemberRepository{db: db}

	// A user rebind that is already in flight must win before member authority
	// is decided. The member mutation locks tenant/member first, then blocks on
	// a fresh users-row lock and recheck; the stale pre-fix EXISTS snapshot
	// returned success here and suspended the target after the actor moved.
	rebind := db.Begin()
	if rebind.Error != nil {
		t.Fatal(rebind.Error)
	}
	if err := rebind.Exec(`UPDATE users SET tenant_id = 2 WHERE id = 'owner'`).Error; err != nil {
		t.Fatal(err)
	}
	rebindResult := make(chan error, 1)
	go func() {
		rebindResult <- repo.UpdateStatus(
			context.Background(), types.MemberActorAuthority{UserID: "owner"},
			"employee", 1, types.TenantMemberStatusSuspended,
		)
	}()
	select {
	case err := <-rebindResult:
		_ = rebind.Rollback().Error
		t.Fatalf("member mutation bypassed in-flight actor rebind: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	if err := rebind.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-rebindResult:
		if !errors.Is(err, ErrMemberActionForbidden) {
			t.Fatalf("actor rebind mutation error = %v, want forbidden", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("member mutation did not finish after actor rebind committed")
	}
	var employeeStatus string
	if err := db.Raw(`SELECT status FROM tenant_members WHERE user_id = 'employee'`).Scan(&employeeStatus).Error; err != nil || employeeStatus != "active" {
		t.Fatalf("employee status=%s err=%v after rejected actor rebind", employeeStatus, err)
	}
	if err := db.Exec(`UPDATE users SET tenant_id = 1 WHERE id = 'owner'`).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Exec(`UPDATE users SET tenant_id = 2 WHERE id = 'employee'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateStatus(context.Background(), types.MemberActorAuthority{UserID: "owner"}, "employee", 1, types.TenantMemberStatusSuspended); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("foreign target mutation error = %v, want not found", err)
	}
	if err := db.Exec(`UPDATE users SET tenant_id = 1 WHERE id = 'employee'`).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Exec(`UPDATE users SET tenant_id = 2 WHERE id = 'admin-b'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.TransferOwnership(context.Background(), "owner", "admin-b", 1); !errors.Is(err, ErrOwnershipTransferInvalid) {
		t.Fatalf("foreign transfer target error = %v, want invalid transfer", err)
	}
	if err := db.Exec(`UPDATE users SET tenant_id = 1 WHERE id = 'admin-b'`).Error; err != nil {
		t.Fatal(err)
	}

	if err := repo.TransferOwnership(context.Background(), "owner", "admin-a", 1); err != nil {
		t.Fatalf("normal transfer: %v", err)
	}
	var roles []struct{ UserID, Role string }
	if err := db.Raw(`SELECT user_id, role FROM tenant_members ORDER BY user_id`).Scan(&roles).Error; err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, row := range roles {
		got[row.UserID] = row.Role
	}
	if got["owner"] != "admin" || got["admin-a"] != "owner" {
		t.Fatalf("normal transfer roles: %#v", got)
	}

	// A transfer and an Owner's stale attempt to suspend its Admin target must
	// serialize on the tenant row. Whichever operation wins, the final state
	// retains exactly one active Owner; after transfer the stale actor may not
	// suspend the newly promoted Owner.
	if err := db.Exec(`DELETE FROM tenant_members WHERE tenant_id = 1`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES
        ('owner', 1, 'owner', 'active'), ('admin-a', 1, 'admin', 'active')`).Error; err != nil {
		t.Fatal(err)
	}
	startSuspend := make(chan struct{})
	var mutationWG sync.WaitGroup
	mutationWG.Add(2)
	go func() {
		defer mutationWG.Done()
		<-startSuspend
		_ = repo.TransferOwnership(context.Background(), "owner", "admin-a", 1)
	}()
	go func() {
		defer mutationWG.Done()
		<-startSuspend
		_ = repo.UpdateStatus(context.Background(), types.MemberActorAuthority{UserID: "owner"}, "admin-a", 1, types.TenantMemberStatusSuspended)
	}()
	close(startSuspend)
	mutationWG.Wait()
	var activeOwners int
	if err := db.Raw(`SELECT count(*) FROM tenant_members WHERE tenant_id = 1 AND role = 'owner' AND status = 'active' AND deleted_at IS NULL`).Scan(&activeOwners).Error; err != nil {
		t.Fatal(err)
	}
	if activeOwners != 1 {
		t.Fatalf("transfer vs suspend active owners=%d, want 1", activeOwners)
	}

	// The old owner can no longer transfer; competing requests serialized by
	// the tenant row therefore yield exactly one successful transfer.
	if err := db.Exec(`DELETE FROM tenant_members WHERE tenant_id = 1`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES
        ('owner', 1, 'owner', 'active'), ('admin-a', 1, 'admin', 'active'),
		('admin-b', 1, 'admin', 'active'), ('employee', 1, 'viewer', 'active')`).Error; err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, target := range []string{"admin-a", "admin-b"} {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			<-start
			results <- repo.TransferOwnership(context.Background(), "owner", target, 1)
		}(target)
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent transfers succeeded %d times, want 1", successes)
	}

	if err := db.Exec(`DELETE FROM tenant_members WHERE tenant_id = 1`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES
		('owner', 1, 'owner', 'active'), ('employee', 1, 'viewer', 'active')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.TransferOwnership(context.Background(), "owner", "employee", 1); !errors.Is(err, ErrOwnershipTransferInvalid) {
		t.Fatalf("non-admin target: %v", err)
	}
	var ownerCount int
	if err := db.Raw(`SELECT count(*) FROM tenant_members WHERE tenant_id = 1 AND role = 'owner' AND status = 'active' AND deleted_at IS NULL`).Scan(&ownerCount).Error; err != nil {
		t.Fatal(err)
	}
	if ownerCount != 1 {
		t.Fatalf("failed transfer must leave one owner, got %d", ownerCount)
	}
	if err := db.Exec(`INSERT INTO tenant_members(user_id, tenant_id, role, status) VALUES ('second-owner', 1, 'owner', 'active')`).Error; err == nil {
		t.Fatal("partial unique index accepted a second active owner")
	}
}
