package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrLastAdministrator is returned by the atomic demote / remove repo helpers
// when the operation would leave the tenant without an active administrator.
// The service layer maps this to its own ErrLastAdministrator sentinel (same
// semantic; just kept separate so the repo doesn't import service).
var (
	ErrLastAdministrator            = errors.New("repository: last active administrator")
	ErrSeatLimitExceeded            = errors.New("repository: enterprise seat limit exceeded")
	ErrUserBoundToAnotherEnterprise = errors.New("repository: user is bound to another enterprise")
	ErrMemberActionForbidden        = errors.New("repository: member action forbidden")
	ErrCannotManageSelf             = errors.New("repository: cannot manage self")
)

func lockTenantAndCheckSeat(ctx context.Context, tx *gorm.DB, tenantID uint64) error {
	var tenant types.Tenant
	if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id", "seats_total").
		Where("id = ?", tenantID).Take(&tenant).Error; err != nil {
		return err
	}
	if tenant.SeatsTotal == nil {
		return nil
	}
	var used int64
	if err := tx.WithContext(ctx).Model(&types.TenantMember{}).
		Where(boundEnterpriseMembership).
		Where("tenant_id = ? AND status = ?", tenantID, types.TenantMemberStatusActive).
		Count(&used).Error; err != nil {
		return err
	}
	if used >= int64(*tenant.SeatsTotal) {
		return ErrSeatLimitExceeded
	}
	return nil
}

// lockActiveTenant is the invitation-acceptance lifecycle boundary. It locks
// before membership inspection so both existing-member and new-member paths
// are ordered with a concurrent enterprise suspension. Capacity remains a
// new-member concern enforced by createTenantMember.
func lockActiveTenant(ctx context.Context, tx *gorm.DB, tenantID uint64) error {
	var tenant types.Tenant
	if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id", "status").
		Where("id = ?", tenantID).Take(&tenant).Error; err != nil {
		return err
	}
	if tenant.Status != types.TenantStatusActive {
		return ErrEnterpriseNotActive
	}
	return nil
}

// forUpdateClause returns the gorm SELECT ... FOR UPDATE clause. Kept
// in one place so we can swap it out for `clause.Locking{Strength: "UPDATE"}`
// on databases that don't support row-level locking (none in our matrix,
// but keeps the seam if SQLite-lite ever needs a no-op).
func forUpdateClause() clause.Expression {
	return clause.Locking{Strength: "UPDATE"}
}

// tenantMemberRepository implements interfaces.TenantMemberRepository.
type tenantMemberRepository struct {
	db *gorm.DB
}

// NewTenantMemberRepository creates a new tenant member repository.
func NewTenantMemberRepository(db *gorm.DB) interfaces.TenantMemberRepository {
	return &tenantMemberRepository{db: db}
}

const boundEnterpriseMembership = `EXISTS (
	SELECT 1 FROM users
	WHERE users.id = tenant_members.user_id
	  AND users.tenant_id = tenant_members.tenant_id
	  AND users.is_active = TRUE
	  AND users.is_system_admin = FALSE
	  AND users.can_access_all_tenants = FALSE
	  AND users.id NOT LIKE 'system-%'
	  AND users.id NOT LIKE 'api_tenant:%'
	  AND users.id NOT LIKE 'api_platform:%'
	  AND users.id NOT LIKE 'api_external_user:%'
	  AND users.deleted_at IS NULL
)`

func createTenantMember(ctx context.Context, tx *gorm.DB, member *types.TenantMember) error {
	if member.Status == "" {
		member.Status = types.TenantMemberStatusActive
	}
	if member.JoinedAt.IsZero() {
		member.JoinedAt = time.Now()
	}
	var tenant types.Tenant
	if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id", "seats_total").
		Where("id = ?", member.TenantID).Take(&tenant).Error; err != nil {
		return err
	}
	var user types.User
	if err := tx.WithContext(ctx).Clauses(forUpdateClause()).
		Select("id", "tenant_id", "is_active", "is_system_admin", "can_access_all_tenants").
		Where("id = ?", member.UserID).Take(&user).Error; err != nil {
		return err
	}
	if !user.IsActive || user.IsSystemAdmin || user.CanAccessAllTenants || isSpecialEnterpriseUserID(user.ID) {
		return ErrMemberActionForbidden
	}
	if user.TenantID != 0 && user.TenantID != member.TenantID {
		return ErrUserBoundToAnotherEnterprise
	}
	if member.Status == types.TenantMemberStatusActive && tenant.SeatsTotal != nil {
		var used int64
		if err := tx.WithContext(ctx).Model(&types.TenantMember{}).
			Where(boundEnterpriseMembership).
			Where("tenant_id = ? AND status = ?", member.TenantID, types.TenantMemberStatusActive).
			Count(&used).Error; err != nil {
			return err
		}
		if used >= int64(*tenant.SeatsTotal) {
			return ErrSeatLimitExceeded
		}
	}
	if user.TenantID == 0 {
		if err := tx.WithContext(ctx).Model(&types.User{}).Where("id = ?", member.UserID).
			Update("tenant_id", member.TenantID).Error; err != nil {
			return err
		}
	}
	return tx.WithContext(ctx).Create(member).Error
}

// lockBoundEnterpriseUser must run after the membership/tenant rows have been
// locked. Under PostgreSQL READ COMMITTED, an EXISTS/JOIN inside the original
// SELECT ... FOR UPDATE can retain a stale third-table snapshot; a fresh
// statement makes the user row itself participate in lock recheck.
func isSpecialEnterpriseUserID(userID string) bool {
	if types.IsSyntheticUserID(userID) {
		return true
	}
	for _, prefix := range []string{types.PrincipalAPITenant + ":", types.PrincipalAPIPlatform + ":", types.PrincipalAPIExternalUser + ":"} {
		if strings.HasPrefix(userID, prefix) {
			return true
		}
	}
	return false
}

func lockBoundEnterpriseUser(ctx context.Context, tx *gorm.DB, userID string, tenantID uint64) error {
	if isSpecialEnterpriseUserID(userID) {
		return ErrMemberActionForbidden
	}
	var user types.User
	err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id").
		Where("id = ? AND tenant_id = ? AND is_active = ? AND is_system_admin = ? AND can_access_all_tenants = ?",
			userID, tenantID, true, false, false).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMemberActionForbidden
	}
	return err
}

// Create binds a tenantless user to the target enterprise and inserts the
// membership in one transaction. Locking the user row serializes competing
// invitations from different enterprises.
func (r *tenantMemberRepository) Create(ctx context.Context, member *types.TenantMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return createTenantMember(ctx, tx, member)
	})
}

// Get returns the active membership for (userID, tenantID), or (nil, nil)
// if no such row exists. Errors are propagated unchanged for any other case.
func (r *tenantMemberRepository) Get(ctx context.Context, userID string, tenantID uint64) (*types.TenantMember, error) {
	var member types.TenantMember
	err := r.db.WithContext(ctx).
		Where(boundEnterpriseMembership).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &member, nil
}

// ListByUser returns every active membership owned by the user, ordered
// by joined_at ascending so the home tenant (created at registration)
// naturally appears first.
func (r *tenantMemberRepository) ListByUser(ctx context.Context, userID string) ([]*types.TenantMember, error) {
	var members []*types.TenantMember
	err := r.db.WithContext(ctx).
		Where(boundEnterpriseMembership).
		Where("user_id = ?", userID).
		Order("joined_at ASC, id ASC").
		Find(&members).Error
	if err != nil {
		return nil, err
	}
	return members, nil
}

// ListByTenant returns every active membership inside the tenant.
func (r *tenantMemberRepository) ListByTenant(ctx context.Context, tenantID uint64) ([]*types.TenantMember, error) {
	var members []*types.TenantMember
	err := r.db.WithContext(ctx).
		Where(boundEnterpriseMembership).
		Where("tenant_id = ?", tenantID).
		Order("joined_at ASC, id ASC").
		Find(&members).Error
	if err != nil {
		return nil, err
	}
	return members, nil
}

// CountFilteredByTenant counts active tenant membership rows, optionally
// restricted to users whose email or username matches search.
func (r *tenantMemberRepository) CountFilteredByTenant(
	ctx context.Context, tenantID uint64, search string,
) (int64, error) {
	search = strings.TrimSpace(search)
	q := r.db.WithContext(ctx).Model(&types.TenantMember{}).
		Joins(`INNER JOIN users ON users.id = tenant_members.user_id AND users.tenant_id = tenant_members.tenant_id AND users.deleted_at IS NULL`).
		Where(boundEnterpriseMembership).
		Where("tenant_members.tenant_id = ?", tenantID)
	var total int64
	var err error
	if search == "" {
		err = q.Count(&total).Error
	} else {
		like := "%" + escapeLikePattern(search) + "%"
		err = q.Where(`(LOWER(users.email) LIKE LOWER(?) OR LOWER(users.username) LIKE LOWER(?))`, like, like).
			Count(&total).Error
	}
	return total, err
}

// ListPagedByTenant lists active memberships with stable sort.
func (r *tenantMemberRepository) ListPagedByTenant(
	ctx context.Context, tenantID uint64, search string, offset, limit int,
) ([]*types.TenantMember, error) {
	search = strings.TrimSpace(search)
	var members []*types.TenantMember
	q := r.db.WithContext(ctx).Model(&types.TenantMember{}).
		Joins(`INNER JOIN users ON users.id = tenant_members.user_id AND users.tenant_id = tenant_members.tenant_id AND users.deleted_at IS NULL`).
		Where(boundEnterpriseMembership).
		Where("tenant_members.tenant_id = ?", tenantID).
		Order("tenant_members.joined_at ASC, tenant_members.id ASC").
		Offset(offset).
		Limit(limit)

	var err error
	if search == "" {
		err = q.Find(&members).Error
	} else {
		like := "%" + escapeLikePattern(search) + "%"
		err = q.Where(`(LOWER(users.email) LIKE LOWER(?) OR LOWER(users.username) LIKE LOWER(?))`, like, like).
			Find(&members).Error
	}
	if err != nil {
		return nil, err
	}
	return members, nil
}

// lockManagedMember serializes every membership member mutation on
// the tenant row, then re-reads the target before applying the actor/target
// policy. This makes a transfer and a concurrent demote/suspend/remove one
// ordered operation rather than two stale preflight checks.
func lockTenantAuthority(ctx context.Context, tx *gorm.DB, actor types.MemberActorAuthority, tenantID uint64) (types.TenantRole, error) {
	var tenant types.Tenant
	if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id", "status").Where("id = ?", tenantID).Take(&tenant).Error; err != nil {
		return "", err
	}
	if tenant.Status != types.TenantStatusActive {
		return "", ErrEnterpriseNotActive
	}
	if actor.SystemAdministrator {
		if actor.UserID == "" {
			return "", ErrMemberActionForbidden
		}
		var user types.User
		if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id").
			Where("id = ? AND is_active = ? AND is_system_admin = ? AND (tenant_id IS NULL OR tenant_id = 0) AND can_access_all_tenants = ?",
				actor.UserID, true, true, false).
			Take(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return "", ErrMemberActionForbidden
			}
			return "", err
		}
		return "", nil
	}
	if actor.ServicePrincipal {
		return "", nil
	}
	if actor.UserID == "" {
		return "", ErrMemberActionForbidden
	}
	var membership types.TenantMember
	if err := tx.WithContext(ctx).Clauses(forUpdateClause()).
		Where("user_id = ? AND tenant_id = ?", actor.UserID, tenantID).Take(&membership).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrMemberActionForbidden
		}
		return "", err
	}
	if membership.Status != types.TenantMemberStatusActive {
		return "", ErrMemberActionForbidden
	}
	if err := lockBoundEnterpriseUser(ctx, tx, membership.UserID, tenantID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrMemberActionForbidden
		}
		return "", err
	}
	return membership.Role, nil
}

func canManage(actor types.MemberActorAuthority, role, target types.TenantRole) bool {
	if actor.ServicePrincipal || actor.SystemAdministrator {
		return target.IsValid()
	}
	return types.CanManageMemberRole(role, target)
}

func lockManagedMember(ctx context.Context, tx *gorm.DB, actor types.MemberActorAuthority, userID string, tenantID uint64, newRole *types.TenantRole) (*types.TenantMember, error) {
	role, err := lockTenantAuthority(ctx, tx, actor, tenantID)
	if err != nil {
		return nil, err
	}
	var target types.TenantMember
	if err := tx.WithContext(ctx).Clauses(forUpdateClause()).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).Take(&target).Error; err != nil {
		return nil, err
	}
	if err := lockBoundEnterpriseUser(ctx, tx, target.UserID, tenantID); err != nil {
		return nil, err
	}
	if !actor.ServicePrincipal && actor.UserID == target.UserID {
		return nil, ErrCannotManageSelf
	}
	if !canManage(actor, role, target.Role) {
		return nil, ErrMemberActionForbidden
	}
	if newRole != nil && !canManage(actor, role, *newRole) {
		return nil, ErrMemberActionForbidden
	}
	return &target, nil
}

func (r *tenantMemberRepository) CreateManaged(ctx context.Context, actor types.MemberActorAuthority, member *types.TenantMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := lockTenantAuthority(ctx, tx, actor, member.TenantID)
		if err != nil {
			return err
		}
		if !canManage(actor, role, member.Role) {
			return ErrMemberActionForbidden
		}
		return createTenantMember(ctx, tx, member)
	})
}

func activeAdministratorCount(ctx context.Context, tx *gorm.DB, tenantID uint64) (int64, error) {
	var count int64
	err := tx.WithContext(ctx).Model(&types.TenantMember{}).Where(boundEnterpriseMembership).
		Where("tenant_id = ? AND role = ? AND status = ?", tenantID, types.TenantRoleAdmin, types.TenantMemberStatusActive).
		Count(&count).Error
	return count, err
}

func (r *tenantMemberRepository) UpdateRole(ctx context.Context, actor types.MemberActorAuthority, userID string, tenantID uint64, role types.TenantRole) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		target, err := lockManagedMember(ctx, tx, actor, userID, tenantID, &role)
		if err != nil {
			return err
		}
		if target.Role == types.TenantRoleAdmin && role != types.TenantRoleAdmin && target.Status == types.TenantMemberStatusActive {
			administrators, err := activeAdministratorCount(ctx, tx, tenantID)
			if err != nil {
				return err
			}
			if administrators <= 1 {
				return ErrLastAdministrator
			}
		}
		res := tx.WithContext(ctx).Model(&types.TenantMember{}).Where("id = ?", target.ID).
			Updates(map[string]any{"role": role, "updated_at": time.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *tenantMemberRepository) UpdateStatus(ctx context.Context, actor types.MemberActorAuthority, userID string, tenantID uint64, status types.TenantMemberStatus) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		target, err := lockManagedMember(ctx, tx, actor, userID, tenantID, nil)
		if err != nil {
			return err
		}
		if target.Status != types.TenantMemberStatusActive && status == types.TenantMemberStatusActive {
			if err := lockTenantAndCheckSeat(ctx, tx, tenantID); err != nil {
				return err
			}
		}
		if target.Role == types.TenantRoleAdmin && target.Status == types.TenantMemberStatusActive && status != types.TenantMemberStatusActive {
			count, err := activeAdministratorCount(ctx, tx, tenantID)
			if err != nil {
				return err
			}
			if count <= 1 {
				return ErrLastAdministrator
			}
		}
		res := tx.WithContext(ctx).Model(&types.TenantMember{}).Where("id = ?", target.ID).
			Updates(map[string]any{"status": status, "updated_at": time.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// UpdateOperatingAnalysisAccess uses its own authority matrix because the
// permission is orthogonal to role lifecycle: Administrators may change any
// member, including themselves and each other.
func (r *tenantMemberRepository) UpdateOperatingAnalysisAccess(
	ctx context.Context,
	actor types.MemberActorAuthority,
	userID string,
	tenantID uint64,
	enabled bool,
) (bool, error) {
	changed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := lockTenantAuthority(ctx, tx, actor, tenantID)
		if err != nil {
			return err
		}
		if !actor.ServicePrincipal && role != types.TenantRoleAdmin {
			return ErrMemberActionForbidden
		}
		var target types.TenantMember
		if err := tx.WithContext(ctx).Clauses(forUpdateClause()).
			Where("user_id = ? AND tenant_id = ?", userID, tenantID).Take(&target).Error; err != nil {
			return err
		}
		if err := lockBoundEnterpriseUser(ctx, tx, target.UserID, tenantID); err != nil {
			return err
		}
		if target.OperatingAnalysisAccess == enabled {
			return nil
		}
		res := tx.WithContext(ctx).Model(&types.TenantMember{}).Where("id = ?", target.ID).
			Updates(map[string]any{"operating_analysis_access": enabled, "updated_at": time.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		changed = true
		return nil
	})
	return changed, err
}

// SoftDelete marks the membership row as deleted after the same tenant lock
// and current actor/target validation used by role and status changes.
func (r *tenantMemberRepository) SoftDelete(ctx context.Context, actor types.MemberActorAuthority, userID string, tenantID uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		target, err := lockManagedMember(ctx, tx, actor, userID, tenantID, nil)
		if err != nil {
			return err
		}
		if target.Role == types.TenantRoleAdmin && target.Status == types.TenantMemberStatusActive {
			administrators, err := activeAdministratorCount(ctx, tx, tenantID)
			if err != nil {
				return err
			}
			if administrators <= 1 {
				return ErrLastAdministrator
			}
		}
		res := tx.WithContext(ctx).Where("id = ?", target.ID).Delete(&types.TenantMember{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// CountActiveAdministrators reports the number of active administrator rows in the tenant.
func (r *tenantMemberRepository) CountActiveAdministrators(ctx context.Context, tenantID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&types.TenantMember{}).
		Where(boundEnterpriseMembership).
		Where("tenant_id = ? AND role = ? AND status = ?",
			tenantID, types.TenantRoleAdmin, types.TenantMemberStatusActive).
		Count(&count).Error
	return count, err
}

// HasAnyMembers reports whether the tenant has an active membership.
func (r *tenantMemberRepository) HasAnyMembers(ctx context.Context, tenantID uint64) (bool, error) {
	var probe struct {
		ID uint64
	}
	err := r.db.WithContext(ctx).
		Model(&types.TenantMember{}).
		Select("id").
		Where(boundEnterpriseMembership).
		Where("tenant_id = ? AND status = ?", tenantID, types.TenantMemberStatusActive).
		Limit(1).
		Take(&probe).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
