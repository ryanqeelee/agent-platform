package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// isDuplicateMembership recognises the unique-constraint violation that
// the tenant_members partial unique index throws when two concurrent
// AddMember / EnsureAdministrator calls race past the in-service Get() check.
// We map this to ErrMembershipAlreadyExists so handlers can return 409
// instead of an opaque 500; the underlying DB still rejects the second
// insert, so this is purely about error-translation, not weakening any
// invariant.
//
// gorm.ErrDuplicatedKey covers the dialect-translated case (gorm ≥1.25
// with TranslateError enabled). The string match on "duplicate" /
// "unique" is the fallback for raw drivers that don't surface the
// sentinel — Postgres "duplicate key value violates unique constraint",
// SQLite "UNIQUE constraint failed", MySQL "Duplicate entry" all
// contain at least one of those tokens.
func isDuplicateMembership(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique constraint")
}

// Sentinel errors returned by tenantMemberService. Callers compare with
// errors.Is to render appropriate HTTP responses (404 / 409 / 403).
var (
	// ErrMembershipNotFound is returned when no active membership row
	// matches the (user, tenant) pair the caller requested.
	ErrMembershipNotFound = errors.New("tenant membership not found")

	// ErrMembershipAlreadyExists is returned by AddMember when the
	// (user, tenant) pair already has an active membership.
	ErrMembershipAlreadyExists = errors.New("tenant membership already exists")

	// ErrUserBoundToAnotherEnterprise keeps one account bound to one company.
	ErrUserBoundToAnotherEnterprise = errors.New("user already belongs to another enterprise")

	// ErrInvalidTenantRole is returned when the caller passes a role
	// value that is not one of the two active tenant roles.
	ErrInvalidTenantRole = errors.New("invalid tenant role")

	// ErrAPIKeyCannotAssignOwner is returned when an API-key principal
	// attempts to persist the Owner role through member or invitation
	// management. manage_members deliberately excludes ownership transfer:
	// a machine principal may manage lower roles, but must never mint a
	// durable human Owner who could subsequently manage API keys or delete
	// the tenant.
	ErrAPIKeyCannotAssignOwner = errors.New("API keys cannot assign the owner role")

	// ErrLastAdministrator is returned when an operation would leave the tenant
	// without an active administrator. Another administrator must remain active.
	ErrLastAdministrator = errors.New("企业至少需要一名有效管理员")

	// ErrMemberActionForbidden means the actor/target pair is outside the
	// enterprise membership lifecycle matrix.
	ErrMemberActionForbidden = errors.New("you cannot manage this member")
	ErrCannotManageSelf      = errors.New("you cannot change your own membership")
	ErrOwnerRoleReserved     = errors.New("owner can only change through ownership transfer")
	ErrInvalidMemberStatus   = errors.New("status must be active or suspended")
)

const (
	listMembersDefaultPageSize = 20
	listMembersMaxPageSize     = 100
)

// tenantMemberService implements interfaces.TenantMemberService.
type tenantMemberService struct {
	repo  interfaces.TenantMemberRepository
	audit interfaces.AuditLogService // optional; nil ⇒ no audit, business ops still succeed
}

// NewTenantMemberService constructs the service. Wired up via the DI
// container alongside the other application services. The auditService
// is optional — passing nil disables durable audit but keeps the
// underlying mutations working, so a container reshuffle that
// constructs tenant_member before audit_log won't crash and tests
// don't need to stub the dependency unless they care about audit
// behaviour.
func NewTenantMemberService(
	repo interfaces.TenantMemberRepository,
	audit interfaces.AuditLogService,
) interfaces.TenantMemberService {
	return &tenantMemberService{repo: repo, audit: audit}
}

// emitAudit is the per-mutation audit hook. Best-effort: a nil audit
// service or a write failure is logged inside the audit service itself
// and never bubbles up to the caller. RBAC mutations succeed even if
// audit is unavailable; the alternative (failing the business op when
// the audit table is down) is far worse.
func (s *tenantMemberService) emitAudit(ctx context.Context, entry *types.AuditLog) {
	if s.audit == nil {
		return
	}
	_ = s.audit.Log(ctx, entry)
}

// auditActorRole picks up the caller's role at write-time. Empty if
// auth middleware didn't set it (e.g. service-internal flows like
// EnsureAdministrator during register, where there is no "caller").
func auditActorRole(ctx context.Context) string {
	return string(types.TenantRoleFromContext(ctx))
}

// auditActor returns the calling user id from context, "" when no
// authenticated caller is present (service-internal paths).
func auditActor(ctx context.Context) string {
	uid, _ := types.UserIDFromContext(ctx)
	return uid
}

func actorRole(ctx context.Context) types.TenantRole {
	return types.TenantRoleFromContext(ctx)
}

func managedActor(ctx context.Context) types.MemberActorAuthority {
	if scope, ok := types.TenantAPIKeyScopeFromContext(ctx); ok &&
		(scope.FullAccess || scope.HasCapability(types.APIKeyCapabilityManageMembers)) {
		return types.MemberActorAuthority{ServicePrincipal: true}
	}
	if types.HasCrossTenantAccessFromContext(ctx) {
		return types.MemberActorAuthority{ServicePrincipal: true}
	}
	id, _ := types.UserIDFromContext(ctx)
	if id == "" {
		// Explicit service-internal callers (invitation acceptance and startup
		// repair) have no request principal; they retain only the machine
		// membership policy, including last-administrator protection.
		return types.MemberActorAuthority{ServicePrincipal: true}
	}
	return types.MemberActorAuthority{UserID: id}
}

func mapMemberMutationError(err error) error {
	switch {
	case errors.Is(err, apprepo.ErrLastAdministrator):
		return ErrLastAdministrator
	case errors.Is(err, apprepo.ErrCannotManageSelf):
		return ErrCannotManageSelf
	case errors.Is(err, apprepo.ErrMemberActionForbidden):
		return ErrMemberActionForbidden
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrMembershipNotFound
	default:
		return err
	}
}

func requireInviteRole(ctx context.Context, role types.TenantRole) error {
	if !role.IsValid() {
		return ErrInvalidTenantRole
	}
	if managedActor(ctx).ServicePrincipal {
		return nil
	}
	if auditActor(ctx) == "" {
		return nil
	}
	if !types.CanInviteMemberRole(actorRole(ctx), role) {
		return ErrMemberActionForbidden
	}
	return nil
}

// AddMember inserts a new active membership row. Returns
// ErrMembershipAlreadyExists if the user is already an active member of
// the tenant, and ErrInvalidTenantRole for unknown roles.
func (s *tenantMemberService) AddMember(
	ctx context.Context,
	userID string,
	tenantID uint64,
	role types.TenantRole,
	invitedBy *string,
) (*types.TenantMember, error) {
	if !role.IsValid() {
		return nil, ErrInvalidTenantRole
	}
	if err := requireInviteRole(ctx, role); err != nil {
		return nil, err
	}
	existing, err := s.repo.Get(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrMembershipAlreadyExists
	}
	member := &types.TenantMember{
		UserID:    userID,
		TenantID:  tenantID,
		Role:      role,
		Status:    types.TenantMemberStatusActive,
		InvitedBy: invitedBy,
		JoinedAt:  time.Now(),
	}
	if err := s.repo.CreateManaged(ctx, managedActor(ctx), member); err != nil {
		if errors.Is(err, apprepo.ErrUserBoundToAnotherEnterprise) {
			return nil, ErrUserBoundToAnotherEnterprise
		}
		// TOCTOU race: a concurrent AddMember / EnsureAdministrator slipped past
		// the Get above. The DB's partial unique index on
		// (user_id, tenant_id) WHERE deleted_at IS NULL caught it; map
		// to the same sentinel the in-service check would have returned
		// so callers get a clean 409 instead of an opaque 500.
		if isDuplicateMembership(err) {
			return nil, ErrMembershipAlreadyExists
		}
		return nil, err
	}
	s.emitAudit(ctx, &types.AuditLog{
		TenantID:     tenantID,
		ActorUserID:  auditActor(ctx),
		ActorRole:    auditActorRole(ctx),
		Action:       types.AuditActionMemberAdded,
		TargetType:   "tenant_member",
		TargetUserID: userID,
		Outcome:      types.AuditOutcomeSuccess,
	})
	return member, nil
}

// EnsureAdministrator is idempotent: if the user already has an active membership
// in the tenant it is returned unchanged; otherwise a new owner row is
// created. Used by Register/OIDC paths so re-running Register on an
// existing user (e.g. after a partial failure) does not double-insert.
func (s *tenantMemberService) EnsureAdministrator(
	ctx context.Context,
	userID string,
	tenantID uint64,
) (*types.TenantMember, error) {
	existing, err := s.repo.Get(ctx, userID, tenantID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	member := &types.TenantMember{
		UserID:   userID,
		TenantID: tenantID,
		Role:     types.TenantRoleAdmin,
		Status:   types.TenantMemberStatusActive,
		JoinedAt: time.Now(),
	}
	if err := s.repo.Create(ctx, member); err != nil {
		if errors.Is(err, apprepo.ErrUserBoundToAnotherEnterprise) {
			return nil, ErrUserBoundToAnotherEnterprise
		}
		// Idempotent contract: if a concurrent Ensure/AddMember beat us
		// (two simultaneous registrations of the same user, or the
		// orphan-tenant self-heal path firing on parallel JWTs), the
		// partial unique index rejects the second insert. Re-read and
		// return the winning row so EnsureAdministrator stays observably
		// idempotent.
		if isDuplicateMembership(err) {
			if winner, getErr := s.repo.Get(ctx, userID, tenantID); getErr == nil && winner != nil {
				logger.Infof(ctx,
					"EnsureAdministrator lost race for user=%s tenant=%d, returning winning row (role=%s)",
					userID, tenantID, winner.Role)
				return winner, nil
			}
		}
		return nil, err
	}
	logger.Infof(ctx, "Bootstrapped owner membership for user=%s tenant=%d", userID, tenantID)
	return member, nil
}

// GetMembership returns the active membership or (nil, nil) when absent.
func (s *tenantMemberService) GetMembership(
	ctx context.Context,
	userID string,
	tenantID uint64,
) (*types.TenantMember, error) {
	return s.repo.Get(ctx, userID, tenantID)
}

// ListByUser proxies to the repository; included on the service so HTTP
// handlers depend only on the service interface.
func (s *tenantMemberService) ListByUser(ctx context.Context, userID string) ([]*types.TenantMember, error) {
	return s.repo.ListByUser(ctx, userID)
}

// ListByTenant proxies to the repository.
func (s *tenantMemberService) ListByTenant(ctx context.Context, tenantID uint64) ([]*types.TenantMember, error) {
	return s.repo.ListByTenant(ctx, tenantID)
}

// ListMembersPage returns a slice plus total matching query (handlers parse
// page/page_size; defensive clamps here mirror list handler limits).
func (s *tenantMemberService) ListMembersPage(
	ctx context.Context,
	tenantID uint64,
	query string,
	page, pageSize int,
) ([]*types.TenantMember, int64, error) {
	query = strings.TrimSpace(query)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = listMembersDefaultPageSize
	}
	if pageSize > listMembersMaxPageSize {
		pageSize = listMembersMaxPageSize
	}
	total, err := s.repo.CountFilteredByTenant(ctx, tenantID, query)
	if err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	members, err := s.repo.ListPagedByTenant(ctx, tenantID, query, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}
	return members, total, nil
}

// HasAnyMembers proxies to the repository.
func (s *tenantMemberService) HasAnyMembers(ctx context.Context, tenantID uint64) (bool, error) {
	return s.repo.HasAnyMembers(ctx, tenantID)
}

// UpdateRole changes an enterprise role under the repository membership policy.
func (s *tenantMemberService) UpdateRole(
	ctx context.Context,
	userID string,
	tenantID uint64,
	newRole types.TenantRole,
) error {
	if !newRole.IsValid() {
		return ErrInvalidTenantRole
	}
	current, err := s.repo.Get(ctx, userID, tenantID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrMembershipNotFound
	}
	oldRole := current.Role
	if err := s.repo.UpdateRole(ctx, managedActor(ctx), userID, tenantID, newRole); err != nil {
		return mapMemberMutationError(err)
	}
	s.emitRoleChangeAudit(ctx, tenantID, userID, oldRole, newRole)
	return nil
}

// emitRoleChangeAudit packs the old/new role into Details so the
// audit-log UI can render "promoted Alice from viewer to admin"
// without a separate column per role transition.
func (s *tenantMemberService) emitRoleChangeAudit(
	ctx context.Context,
	tenantID uint64,
	targetUserID string,
	oldRole, newRole types.TenantRole,
) {
	details, _ := json.Marshal(map[string]string{
		"old_role": string(oldRole),
		"new_role": string(newRole),
	})
	s.emitAudit(ctx, &types.AuditLog{
		TenantID:     tenantID,
		ActorUserID:  auditActor(ctx),
		ActorRole:    auditActorRole(ctx),
		Action:       types.AuditActionMemberRoleChanged,
		TargetType:   "tenant_member",
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
		Details:      types.JSON(details),
	})
}

// RemoveMember is the administrative soft-delete path. Repository checks
// preserve at least one active administrator.
//
// The audit row distinguishes "voluntary leave" (caller == target,
// driven by POST /leave) from "kicked" (caller != target, driven by
// DELETE /tenants/:id/members/:user_id). Both go through this same
// service method but the recorded action differs so an audit reader
// can tell the two apart.
func (s *tenantMemberService) RemoveMember(ctx context.Context, userID string, tenantID uint64) error {
	current, err := s.repo.Get(ctx, userID, tenantID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrMembershipNotFound
	}
	if err := s.repo.SoftDelete(ctx, managedActor(ctx), userID, tenantID); err != nil {
		return mapMemberMutationError(err)
	}
	s.emitRemovalAudit(ctx, tenantID, userID)
	return nil
}

// LeaveTenant is the only self-removal path. It deliberately skips the
// administrator target matrix but preserves the last-administrator invariant.
func (s *tenantMemberService) LeaveTenant(ctx context.Context, userID string, tenantID uint64) error {
	current, err := s.repo.Get(ctx, userID, tenantID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrMembershipNotFound
	}
	if err := s.repo.SoftDelete(ctx, types.MemberActorAuthority{ServicePrincipal: true}, userID, tenantID); err != nil {
		return mapMemberMutationError(err)
	}
	s.emitRemovalAudit(ctx, tenantID, userID)
	return nil
}

func (s *tenantMemberService) UpdateStatus(
	ctx context.Context,
	userID string,
	tenantID uint64,
	status types.TenantMemberStatus,
) error {
	if status != types.TenantMemberStatusActive && status != types.TenantMemberStatusSuspended {
		return ErrInvalidMemberStatus
	}
	member, err := s.repo.Get(ctx, userID, tenantID)
	if err != nil {
		return err
	}
	if member == nil {
		return ErrMembershipNotFound
	}
	if err := s.repo.UpdateStatus(ctx, managedActor(ctx), userID, tenantID, status); err != nil {
		return mapMemberMutationError(err)
	}
	s.emitAudit(ctx, &types.AuditLog{
		TenantID: tenantID, ActorUserID: auditActor(ctx), ActorRole: auditActorRole(ctx),
		Action: types.AuditActionMemberStatusChanged, TargetType: "tenant_member", TargetUserID: userID,
		Outcome: types.AuditOutcomeSuccess,
	})
	return nil
}

func (s *tenantMemberService) UpdateOperatingAnalysisAccess(
	ctx context.Context,
	userID string,
	tenantID uint64,
	enabled bool,
) error {
	changed, err := s.repo.UpdateOperatingAnalysisAccess(
		ctx, managedActor(ctx), userID, tenantID, enabled,
	)
	if err != nil {
		return mapMemberMutationError(err)
	}
	if !changed {
		return nil
	}
	details, _ := json.Marshal(map[string]bool{
		"old_enabled": !enabled,
		"new_enabled": enabled,
	})
	s.emitAudit(ctx, &types.AuditLog{
		TenantID: tenantID, ActorUserID: auditActor(ctx), ActorRole: auditActorRole(ctx),
		Action:     types.AuditActionOperatingAnalysisAccessChanged,
		TargetType: "tenant_member", TargetUserID: userID,
		Outcome: types.AuditOutcomeSuccess, Details: types.JSON(details),
	})
	return nil
}

func (s *tenantMemberService) emitRemovalAudit(
	ctx context.Context,
	tenantID uint64,
	targetUserID string,
) {
	action := types.AuditActionMemberRemoved
	if actor := auditActor(ctx); actor != "" && actor == targetUserID {
		action = types.AuditActionMemberLeft
	}
	s.emitAudit(ctx, &types.AuditLog{
		TenantID:     tenantID,
		ActorUserID:  auditActor(ctx),
		ActorRole:    auditActorRole(ctx),
		Action:       action,
		TargetType:   "tenant_member",
		TargetUserID: targetUserID,
		Outcome:      types.AuditOutcomeSuccess,
	})
}
