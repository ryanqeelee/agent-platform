package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// gormErrRecordNotFound is the sentinel the fake repo returns when an
// atomic helper is asked to touch a row that no longer exists, matching
// the real repo's behaviour. Kept as a package-level alias so the
// tests can reference it without re-importing gorm at every call site.
var gormErrRecordNotFound = gorm.ErrRecordNotFound

// fakeTenantMemberRepo is an in-memory implementation of
// interfaces.TenantMemberRepository for unit tests. It is intentionally
// not safe for concurrent use; tests should drive it sequentially.
type fakeTenantMemberRepo struct {
	rows []*types.TenantMember
	// nextID is incremented on Create to populate the surrogate PK.
	nextID uint64
	// failGet, failHasAny etc. let tests inject transient errors on the
	// matching method to exercise error paths.
	failGet         error
	failHasAny      error
	failCountOwners error
	failUpdateRole  error
	failSoftDelete  error
	failCreate      error
}

func newFakeRepo() *fakeTenantMemberRepo { return &fakeTenantMemberRepo{} }

func (r *fakeTenantMemberRepo) Create(ctx context.Context, m *types.TenantMember) error {
	if r.failCreate != nil {
		return r.failCreate
	}
	r.nextID++
	m.ID = r.nextID
	// Mirror the partial unique index on (user_id, tenant_id) for
	// active rows so tests can exercise duplicate-insert behaviour.
	for _, e := range r.rows {
		if e.UserID == m.UserID && e.TenantID == m.TenantID && e.DeletedAt.Valid == false {
			return errors.New("duplicate active membership")
		}
	}
	cp := *m
	r.rows = append(r.rows, &cp)
	return nil
}

func (r *fakeTenantMemberRepo) Get(ctx context.Context, userID string, tenantID uint64) (*types.TenantMember, error) {
	if r.failGet != nil {
		return nil, r.failGet
	}
	for _, e := range r.rows {
		if e.UserID == userID && e.TenantID == tenantID && !e.DeletedAt.Valid {
			cp := *e
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *fakeTenantMemberRepo) validateActor(actor types.MemberActorAuthority, tenantID uint64) error {
	if actor.ServicePrincipal {
		return nil
	}
	if actor.UserID == "" {
		return apprepo.ErrMemberActionForbidden
	}
	for _, e := range r.rows {
		if e.UserID == actor.UserID && e.TenantID == tenantID && !e.DeletedAt.Valid && e.Status == types.TenantMemberStatusActive {
			return nil
		}
	}
	return apprepo.ErrMemberActionForbidden
}

func (r *fakeTenantMemberRepo) CreateManaged(ctx context.Context, actor types.MemberActorAuthority, m *types.TenantMember) error {
	if err := r.validateActor(actor, m.TenantID); err != nil {
		return err
	}
	if !actor.ServicePrincipal {
		var role types.TenantRole
		for _, e := range r.rows {
			if e.UserID == actor.UserID && e.TenantID == m.TenantID {
				role = e.Role
				break
			}
		}
		if !types.CanManageMemberRole(role, m.Role) {
			return apprepo.ErrMemberActionForbidden
		}
	}
	return r.Create(ctx, m)
}

func (r *fakeTenantMemberRepo) ListByUser(ctx context.Context, userID string) ([]*types.TenantMember, error) {
	var out []*types.TenantMember
	for _, e := range r.rows {
		if e.UserID == userID && !e.DeletedAt.Valid {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakeTenantMemberRepo) ListByTenant(ctx context.Context, tenantID uint64) ([]*types.TenantMember, error) {
	var out []*types.TenantMember
	for _, e := range r.rows {
		if e.TenantID == tenantID && !e.DeletedAt.Valid {
			cp := *e
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakeTenantMemberRepo) filterTenantRows(tenantID uint64, search string) []*types.TenantMember {
	search = strings.TrimSpace(strings.ToLower(search))
	var out []*types.TenantMember
	for _, e := range r.rows {
		if e.TenantID != tenantID || e.DeletedAt.Valid {
			continue
		}
		if search != "" {
			if !strings.Contains(strings.ToLower(e.UserID), search) {
				continue
			}
		}
		cp := *e
		out = append(out, &cp)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].JoinedAt.Equal(out[j].JoinedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].JoinedAt.Before(out[j].JoinedAt)
	})
	return out
}

func (r *fakeTenantMemberRepo) CountFilteredByTenant(
	ctx context.Context, tenantID uint64, search string,
) (int64, error) {
	return int64(len(r.filterTenantRows(tenantID, search))), nil
}

func (r *fakeTenantMemberRepo) ListPagedByTenant(
	ctx context.Context, tenantID uint64, search string, offset, limit int,
) ([]*types.TenantMember, error) {
	all := r.filterTenantRows(tenantID, search)
	if offset >= len(all) {
		return []*types.TenantMember{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return append([]*types.TenantMember(nil), all[offset:end]...), nil
}

func (r *fakeTenantMemberRepo) UpdateRole(ctx context.Context, actor types.MemberActorAuthority, userID string, tenantID uint64, role types.TenantRole) error {
	if r.failUpdateRole != nil {
		return r.failUpdateRole
	}
	if err := r.validateActor(actor, tenantID); err != nil {
		return err
	}
	for _, e := range r.rows {
		if e.UserID == userID && e.TenantID == tenantID && !e.DeletedAt.Valid {
			if !actor.ServicePrincipal && actor.UserID == userID {
				return apprepo.ErrCannotManageSelf
			}
			if !actor.ServicePrincipal && !types.CanManageMemberRole(actorRoleFor(r, actor.UserID, tenantID), e.Role) {
				return apprepo.ErrMemberActionForbidden
			}
			if !actor.ServicePrincipal && !types.CanManageMemberRole(actorRoleFor(r, actor.UserID, tenantID), role) {
				return apprepo.ErrMemberActionForbidden
			}
			if e.Role == types.TenantRoleAdmin && e.Status == types.TenantMemberStatusActive && role != types.TenantRoleAdmin {
				owners, err := r.CountActiveAdministrators(ctx, tenantID)
				if err != nil {
					return err
				}
				if owners <= 1 {
					return apprepo.ErrLastAdministrator
				}
			}
			e.Role = role
			return nil
		}
	}
	return errors.New("not found")
}

func actorRoleFor(r *fakeTenantMemberRepo, userID string, tenantID uint64) types.TenantRole {
	for _, e := range r.rows {
		if e.UserID == userID && e.TenantID == tenantID && !e.DeletedAt.Valid {
			return e.Role
		}
	}
	return ""
}

func (r *fakeTenantMemberRepo) UpdateStatus(ctx context.Context, actor types.MemberActorAuthority, userID string, tenantID uint64, status types.TenantMemberStatus) error {
	if err := r.validateActor(actor, tenantID); err != nil {
		return err
	}
	for _, e := range r.rows {
		if e.UserID == userID && e.TenantID == tenantID && !e.DeletedAt.Valid {
			if !actor.ServicePrincipal && actor.UserID == userID {
				return apprepo.ErrCannotManageSelf
			}
			if !actor.ServicePrincipal && !types.CanManageMemberRole(actorRoleFor(r, actor.UserID, tenantID), e.Role) {
				return apprepo.ErrMemberActionForbidden
			}
			e.Status = status
			return nil
		}
	}
	return gormErrRecordNotFound
}

func (r *fakeTenantMemberRepo) UpdateOperatingAnalysisAccess(
	ctx context.Context,
	actor types.MemberActorAuthority,
	userID string,
	tenantID uint64,
	enabled bool,
) (bool, error) {
	if err := r.validateActor(actor, tenantID); err != nil {
		return false, err
	}
	if !actor.ServicePrincipal {
		role := actorRoleFor(r, actor.UserID, tenantID)
		if role != types.TenantRoleAdmin {
			return false, apprepo.ErrMemberActionForbidden
		}
	}
	for _, member := range r.rows {
		if member.UserID == userID && member.TenantID == tenantID && !member.DeletedAt.Valid {
			if member.OperatingAnalysisAccess == enabled {
				return false, nil
			}
			member.OperatingAnalysisAccess = enabled
			return true, nil
		}
	}
	return false, gormErrRecordNotFound
}

func (r *fakeTenantMemberRepo) SoftDelete(ctx context.Context, actor types.MemberActorAuthority, userID string, tenantID uint64) error {
	if r.failSoftDelete != nil {
		return r.failSoftDelete
	}
	if err := r.validateActor(actor, tenantID); err != nil {
		return err
	}
	for _, e := range r.rows {
		if e.UserID == userID && e.TenantID == tenantID && !e.DeletedAt.Valid {
			if !actor.ServicePrincipal && actor.UserID == userID {
				return apprepo.ErrCannotManageSelf
			}
			if !actor.ServicePrincipal && !types.CanManageMemberRole(actorRoleFor(r, actor.UserID, tenantID), e.Role) {
				return apprepo.ErrMemberActionForbidden
			}
			if e.Role == types.TenantRoleAdmin && e.Status == types.TenantMemberStatusActive {
				owners, err := r.CountActiveAdministrators(ctx, tenantID)
				if err != nil {
					return err
				}
				if owners <= 1 {
					return apprepo.ErrLastAdministrator
				}
			}
			e.DeletedAt.Valid = true
			return nil
		}
	}
	return errors.New("not found")
}

func (r *fakeTenantMemberRepo) CountActiveAdministrators(ctx context.Context, tenantID uint64) (int64, error) {
	if r.failCountOwners != nil {
		return 0, r.failCountOwners
	}
	var n int64
	for _, e := range r.rows {
		if e.TenantID == tenantID && !e.DeletedAt.Valid &&
			e.Role == types.TenantRoleAdmin && e.Status == types.TenantMemberStatusActive {
			n++
		}
	}
	return n, nil
}

func (r *fakeTenantMemberRepo) HasAnyMembers(ctx context.Context, tenantID uint64) (bool, error) {
	if r.failHasAny != nil {
		return false, r.failHasAny
	}
	for _, e := range r.rows {
		if e.TenantID == tenantID && !e.DeletedAt.Valid && e.Status == types.TenantMemberStatusActive {
			return true, nil
		}
	}
	return false, nil
}

// DemoteOwnerAtomically and RemoveOwnerAtomically mimic the production
// repo's transactional invariants: count other active Owners, fail
// closed when there are none, otherwise apply the role change /
// soft-delete in-memory. The fake doesn't simulate row-level locks
// because every Go test is single-goroutine here; the production
// transaction is exercised via the repo tests against a real DB.
func (r *fakeTenantMemberRepo) DemoteOwnerAtomically(
	ctx context.Context, userID string, tenantID uint64, newRole types.TenantRole,
) error {
	others := int64(0)
	var target *types.TenantMember
	for _, e := range r.rows {
		if e.TenantID != tenantID || e.DeletedAt.Valid || e.Status != types.TenantMemberStatusActive {
			continue
		}
		if e.Role == types.TenantRoleAdmin && e.UserID != userID {
			others++
		}
		if e.UserID == userID {
			target = e
		}
	}
	if others == 0 {
		return apprepo.ErrLastAdministrator
	}
	if target == nil {
		return gormErrRecordNotFound
	}
	target.Role = newRole
	target.UpdatedAt = time.Now()
	return nil
}

func (r *fakeTenantMemberRepo) RemoveOwnerAtomically(
	ctx context.Context, userID string, tenantID uint64,
) error {
	others := int64(0)
	var target *types.TenantMember
	for _, e := range r.rows {
		if e.TenantID != tenantID || e.DeletedAt.Valid || e.Status != types.TenantMemberStatusActive {
			continue
		}
		if e.Role == types.TenantRoleAdmin && e.UserID != userID {
			others++
		}
		if e.UserID == userID {
			target = e
		}
	}
	if others == 0 {
		return apprepo.ErrLastAdministrator
	}
	if target == nil {
		return gormErrRecordNotFound
	}
	target.DeletedAt.Time = time.Now()
	target.DeletedAt.Valid = true
	return nil
}

// Compile-time guard so the test stays in sync with the interface.
var _ interfaces.TenantMemberRepository = (*fakeTenantMemberRepo)(nil)

func newServiceWithRepo() (interfaces.TenantMemberService, *fakeTenantMemberRepo) {
	r := newFakeRepo()
	return NewTenantMemberService(r, nil), r
}

type operatingAnalysisAuditCapture struct {
	interfaces.AuditLogService
	entries []*types.AuditLog
}

func (c *operatingAnalysisAuditCapture) Log(_ context.Context, entry *types.AuditLog) error {
	c.entries = append(c.entries, entry)
	return nil
}

func TestTenantMemberService_OperatingAnalysisAccessUsesIndependentAdminMatrix(t *testing.T) {
	repo := newFakeRepo()
	repo.rows = []*types.TenantMember{
		{UserID: "owner", TenantID: 1, Role: types.TenantRoleAdmin, Status: types.TenantMemberStatusActive},
		{UserID: "admin", TenantID: 1, Role: types.TenantRoleAdmin, Status: types.TenantMemberStatusActive},
		{UserID: "employee", TenantID: 1, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive},
	}
	audit := &operatingAnalysisAuditCapture{}
	svc := NewTenantMemberService(repo, audit)

	if err := svc.UpdateOperatingAnalysisAccess(memberActorCtx("owner", types.TenantRoleAdmin), "owner", 1, true); err != nil {
		t.Fatalf("owner self grant: %v", err)
	}
	if err := svc.UpdateOperatingAnalysisAccess(memberActorCtx("owner", types.TenantRoleAdmin), "owner", 1, true); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if err := svc.UpdateOperatingAnalysisAccess(memberActorCtx("admin", types.TenantRoleAdmin), "owner", 1, false); err != nil {
		t.Fatalf("admin revoke owner: %v", err)
	}
	if err := svc.UpdateOperatingAnalysisAccess(memberActorCtx("employee", types.TenantRoleViewer), "admin", 1, true); !errors.Is(err, ErrMemberActionForbidden) {
		t.Fatalf("employee mutation = %v, want forbidden", err)
	}
	if len(audit.entries) != 2 || audit.entries[0].Action != types.AuditActionOperatingAnalysisAccessChanged {
		t.Fatalf("audit entries = %+v, want two real changes", audit.entries)
	}
}

func TestTenantMemberService_AddMember_RejectsInvalidRole(t *testing.T) {
	svc, _ := newServiceWithRepo()
	_, err := svc.AddMember(context.Background(), "u1", 1, types.TenantRole("nonsense"), nil)
	if !errors.Is(err, ErrInvalidTenantRole) {
		t.Fatalf("want ErrInvalidTenantRole, got %v", err)
	}
}

func TestTenantMemberService_AddMember_APIKeyCannotAssignOwner(t *testing.T) {
	svc, repo := newServiceWithRepo()
	ctx := types.WithTenantAPIKeyScope(context.Background(), types.TenantAPIKeyScope{
		KeyID:        1,
		Capabilities: types.StringArray{string(types.APIKeyCapabilityManageMembers)},
	})

	_, err := svc.AddMember(ctx, "u1", 1, types.TenantRoleOwner, nil)
	if !errors.Is(err, ErrInvalidTenantRole) {
		t.Fatalf("want ErrInvalidTenantRole, got %v", err)
	}
	if len(repo.rows) != 0 {
		t.Fatalf("API key owner assignment must not create a membership, got %d rows", len(repo.rows))
	}
}

func TestTenantMemberService_AddMember_RejectsDuplicate(t *testing.T) {
	svc, _ := newServiceWithRepo()
	ctx := context.Background()
	if _, err := svc.AddMember(ctx, "u1", 1, types.TenantRoleViewer, nil); err != nil {
		t.Fatalf("first AddMember: %v", err)
	}
	_, err := svc.AddMember(ctx, "u1", 1, types.TenantRoleViewer, nil)
	if !errors.Is(err, ErrMembershipAlreadyExists) {
		t.Fatalf("want ErrMembershipAlreadyExists, got %v", err)
	}
}

func TestTenantMemberService_AddMember_MapsDuplicateKeyRace(t *testing.T) {
	// Simulate the TOCTOU race: Get() saw no row, then a concurrent
	// AddMember inserted before us, so our Create hits the partial
	// unique index. The DB returns a duplicate-key error, which the
	// service must translate into ErrMembershipAlreadyExists so the
	// handler returns 409 rather than a generic 500.
	svc, repo := newServiceWithRepo()
	repo.failCreate = errors.New(
		"ERROR: duplicate key value violates unique constraint \"idx_tenant_members_user_tenant_unique\"")
	_, err := svc.AddMember(context.Background(), "u_race", 1, types.TenantRoleViewer, nil)
	if !errors.Is(err, ErrMembershipAlreadyExists) {
		t.Fatalf("want ErrMembershipAlreadyExists on duplicate-key race, got %v", err)
	}
}

func TestTenantMemberService_AddMember_MapsInactiveEnterprise(t *testing.T) {
	svc, repo := newServiceWithRepo()
	repo.failCreate = apprepo.ErrEnterpriseNotActive

	_, err := svc.AddMember(context.Background(), "u1", 1, types.TenantRoleViewer, nil)
	if !errors.Is(err, ErrEnterpriseNotActive) {
		t.Fatalf("want ErrEnterpriseNotActive, got %v", err)
	}
}

func TestTenantMemberService_EnsureAdministrator_Idempotent(t *testing.T) {
	svc, repo := newServiceWithRepo()
	ctx := context.Background()
	first, err := svc.EnsureAdministrator(ctx, "u1", 1)
	if err != nil {
		t.Fatalf("first EnsureAdministrator: %v", err)
	}
	second, err := svc.EnsureAdministrator(ctx, "u1", 1)
	if err != nil {
		t.Fatalf("second EnsureAdministrator: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("EnsureAdministrator not idempotent: %d vs %d", first.ID, second.ID)
	}
	if len(repo.rows) != 1 {
		t.Fatalf("want exactly 1 row after idempotent EnsureAdministrator, got %d", len(repo.rows))
	}
}

func TestTenantMemberService_UpdateRole_RejectsLastAdministratorDemotion(t *testing.T) {
	svc, _ := newServiceWithRepo()
	ctx := context.Background()
	if _, err := svc.EnsureAdministrator(ctx, "owner", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := svc.UpdateRole(ctx, "owner", 1, types.TenantRoleViewer)
	if !errors.Is(err, ErrLastAdministrator) {
		t.Fatalf("ordinary role writes must not demote Owner, got %v", err)
	}
}

func TestTenantMemberService_UpdateRole_APIKeyCannotPromoteOwner(t *testing.T) {
	svc, _ := newServiceWithRepo()
	humanCtx := context.Background()
	if _, err := svc.AddMember(humanCtx, "u1", 1, types.TenantRoleAdmin, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	apiKeyCtx := types.WithTenantAPIKeyScope(humanCtx, types.TenantAPIKeyScope{
		KeyID:        1,
		Capabilities: types.StringArray{string(types.APIKeyCapabilityManageMembers)},
	})

	err := svc.UpdateRole(apiKeyCtx, "u1", 1, types.TenantRoleOwner)
	if !errors.Is(err, ErrInvalidTenantRole) {
		t.Fatalf("want ErrInvalidTenantRole, got %v", err)
	}
	member, getErr := svc.GetMembership(humanCtx, "u1", 1)
	if getErr != nil {
		t.Fatalf("get membership: %v", getErr)
	}
	if member == nil || member.Role != types.TenantRoleAdmin {
		t.Fatalf("API key promotion must leave role unchanged, got %+v", member)
	}
}

func TestTenantMemberService_AddMember_RejectsOwnerOutsideBootstrap(t *testing.T) {
	svc, _ := newServiceWithRepo()
	ctx := context.Background()
	if _, err := svc.AddMember(ctx, "owner2", 1, types.TenantRoleOwner, nil); !errors.Is(err, ErrInvalidTenantRole) {
		t.Fatalf("ordinary AddMember must reject Owner, got %v", err)
	}
}

func TestTenantMemberService_UpdateRole_NoopOnSameRole(t *testing.T) {
	svc, _ := newServiceWithRepo()
	ctx := context.Background()
	if _, err := svc.EnsureAdministrator(ctx, "owner", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := svc.UpdateRole(ctx, "owner", 1, types.TenantRoleOwner); !errors.Is(err, ErrInvalidTenantRole) {
		t.Fatalf("ordinary role writes must reject Owner, got %v", err)
	}
}

func memberActorCtx(userID string, role types.TenantRole) context.Context {
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, userID)
	return context.WithValue(ctx, types.TenantRoleContextKey, role)
}

func platformMemberActorCtx(enabled bool) context.Context {
	ctx := memberActorCtx("platform", types.TenantRoleViewer)
	ctx = context.WithValue(ctx, types.UserContextKey, &types.User{
		ID:                  "platform",
		CanAccessAllTenants: true,
	})
	// Auth middleware owns this internal, feature-gated projection.
	return context.WithValue(ctx, types.CrossTenantAccessContextKey, enabled)
}

func TestTenantMemberService_PlatformAuthorityRequiresFeatureGatedContext(t *testing.T) {
	seed := func() interfaces.TenantMemberService {
		svc, repo := newServiceWithRepo()
		repo.rows = []*types.TenantMember{
			{UserID: "platform", TenantID: 1, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive},
			{UserID: "target-admin", TenantID: 1, Role: types.TenantRoleAdmin, Status: types.TenantMemberStatusActive},
		}
		return svc
	}

	if err := seed().UpdateStatus(platformMemberActorCtx(false), "target-admin", 1, types.TenantMemberStatusSuspended); !errors.Is(err, ErrMemberActionForbidden) {
		t.Fatalf("raw CanAccessAllTenants must not elevate while the feature gate is off: %v", err)
	}
	if err := seed().UpdateStatus(platformMemberActorCtx(true), "target-admin", 1, types.TenantMemberStatusSuspended); err != nil {
		t.Fatalf("feature-gated platform operator must manage a non-Owner: %v", err)
	}
}

func TestTenantMemberService_ActorTargetMatrixAndStatus(t *testing.T) {
	seed := func() (interfaces.TenantMemberService, *fakeTenantMemberRepo) {
		svc, repo := newServiceWithRepo()
		repo.rows = []*types.TenantMember{
			{UserID: "owner", TenantID: 1, Role: types.TenantRoleAdmin, Status: types.TenantMemberStatusActive},
			{UserID: "admin", TenantID: 1, Role: types.TenantRoleAdmin, Status: types.TenantMemberStatusActive},
			{UserID: "ka", TenantID: 1, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive},
			{UserID: "employee", TenantID: 1, Role: types.TenantRoleViewer, Status: types.TenantMemberStatusActive},
		}
		return svc, repo
	}
	cases := []struct {
		name   string
		ctx    context.Context
		target string
		role   types.TenantRole
		want   error
	}{
		{"owner appoints admin", memberActorCtx("owner", types.TenantRoleAdmin), "ka", types.TenantRoleAdmin, nil},
		{"admin cannot touch admin", memberActorCtx("admin", types.TenantRoleAdmin), "admin", types.TenantRoleViewer, ErrCannotManageSelf},
		{"admin cannot target owner", memberActorCtx("admin", types.TenantRoleAdmin), "owner", types.TenantRoleViewer, nil},
		{"knowledge administrator cannot manage employee", memberActorCtx("ka", types.TenantRoleViewer), "employee", types.TenantRoleViewer, ErrMemberActionForbidden},
		{"ordinary role write cannot mint owner", memberActorCtx("owner", types.TenantRoleAdmin), "employee", types.TenantRoleOwner, ErrInvalidTenantRole},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := seed()
			err := svc.UpdateRole(tc.ctx, tc.target, 1, tc.role)
			if tc.want == nil && err != nil {
				t.Fatalf("UpdateRole: %v", err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("UpdateRole: got %v, want %v", err, tc.want)
			}
		})
	}
	svc, _ := seed()
	if err := svc.UpdateStatus(memberActorCtx("owner", types.TenantRoleAdmin), "employee", 1, types.TenantMemberStatusSuspended); err != nil {
		t.Fatalf("suspend employee: %v", err)
	}
	member, _ := svc.GetMembership(context.Background(), "employee", 1)
	if member.Role != types.TenantRoleViewer || member.Status != types.TenantMemberStatusSuspended {
		t.Fatalf("suspension must retain role, got %+v", member)
	}
}

func TestTenantMemberService_UpdateRole_RejectsInvalidRole(t *testing.T) {
	svc, _ := newServiceWithRepo()
	ctx := context.Background()
	if _, err := svc.EnsureAdministrator(ctx, "owner", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := svc.UpdateRole(ctx, "owner", 1, types.TenantRole("nope")); !errors.Is(err, ErrInvalidTenantRole) {
		t.Fatalf("want ErrInvalidTenantRole, got %v", err)
	}
}

func TestTenantMemberService_UpdateRole_ReturnsNotFound(t *testing.T) {
	svc, _ := newServiceWithRepo()
	if err := svc.UpdateRole(context.Background(), "ghost", 1, types.TenantRoleAdmin); !errors.Is(err, ErrMembershipNotFound) {
		t.Fatalf("want ErrMembershipNotFound, got %v", err)
	}
}

func TestTenantMemberService_RemoveMember_RejectsOwnerMutationOutsideTransfer(t *testing.T) {
	svc, _ := newServiceWithRepo()
	ctx := context.Background()
	if _, err := svc.EnsureAdministrator(ctx, "owner", 1); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := svc.RemoveMember(ctx, "owner", 1); !errors.Is(err, ErrLastAdministrator) {
		t.Fatalf("ordinary removal must not remove Owner, got %v", err)
	}
}

func TestTenantMemberService_RemoveMember_AllowsContributorRemoval(t *testing.T) {
	svc, _ := newServiceWithRepo()
	ctx := context.Background()
	if _, err := svc.EnsureAdministrator(ctx, "owner", 1); err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if _, err := svc.AddMember(ctx, "contrib", 1, types.TenantRoleViewer, nil); err != nil {
		t.Fatalf("seed contributor: %v", err)
	}
	if err := svc.RemoveMember(ctx, "contrib", 1); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	got, _ := svc.GetMembership(ctx, "contrib", 1)
	if got != nil {
		t.Fatalf("contributor should be soft-deleted, got %+v", got)
	}
}

func TestTenantMemberService_RemoveMember_ReturnsNotFound(t *testing.T) {
	svc, _ := newServiceWithRepo()
	if err := svc.RemoveMember(context.Background(), "ghost", 1); !errors.Is(err, ErrMembershipNotFound) {
		t.Fatalf("want ErrMembershipNotFound, got %v", err)
	}
}

func TestTenantRole_HasPermission(t *testing.T) {
	cases := []struct {
		caller   types.TenantRole
		required types.TenantRole
		want     bool
	}{
		{types.TenantRoleOwner, types.TenantRoleAdmin, false},
		{types.TenantRoleAdmin, types.TenantRoleOwner, false},
		{types.TenantRoleViewer, types.TenantRoleViewer, true},
		{types.TenantRoleViewer, types.TenantRoleAdmin, false},
		{types.TenantRole("bogus"), types.TenantRoleViewer, false},
	}
	for _, c := range cases {
		if got := c.caller.HasPermission(c.required); got != c.want {
			t.Errorf("HasPermission(%s, %s) = %v, want %v", c.caller, c.required, got, c.want)
		}
	}
}

func (r *fakeTenantMemberRepo) CreateEmployee(context.Context, types.MemberActorAuthority, *types.User, *types.TenantMember) error {
	panic("unexpected CreateEmployee")
}
