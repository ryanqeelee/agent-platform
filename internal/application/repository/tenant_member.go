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

// ErrLastOwner is returned by the atomic demote / remove repo helpers
// when the operation would leave the tenant without an active Owner.
// The service layer maps this to its own ErrLastOwner sentinel (same
// semantic; just kept separate so the repo doesn't import service).
var (
	ErrLastOwner                    = errors.New("repository: last active owner")
	ErrUserBoundToAnotherEnterprise = errors.New("repository: user is bound to another enterprise")
	ErrOwnershipTransferInvalid     = errors.New("repository: ownership transfer precondition failed")
	ErrOwnershipInvariant           = errors.New("repository: expected exactly one active owner")
	ErrMemberActionForbidden        = errors.New("repository: member action forbidden")
	ErrCannotManageSelf             = errors.New("repository: cannot manage self")
)

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
	  AND users.deleted_at IS NULL
)`

func createTenantMember(ctx context.Context, tx *gorm.DB, member *types.TenantMember) error {
	if member.Status == "" {
		member.Status = types.TenantMemberStatusActive
	}
	if member.JoinedAt.IsZero() {
		member.JoinedAt = time.Now()
	}
	var user types.User
	if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id", "tenant_id").
		Where("id = ?", member.UserID).Take(&user).Error; err != nil {
		return err
	}
	if user.TenantID != 0 && user.TenantID != member.TenantID {
		return ErrUserBoundToAnotherEnterprise
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
func lockBoundEnterpriseUser(ctx context.Context, tx *gorm.DB, userID string, tenantID uint64) error {
	var user types.User
	return tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id").
		Where("id = ? AND tenant_id = ?", userID, tenantID).Take(&user).Error
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

// lockManagedMember serializes every ownership-affecting member mutation on
// the tenant row, then re-reads the target before applying the actor/target
// policy. This makes a transfer and a concurrent demote/suspend/remove one
// ordered operation rather than two stale preflight checks.
func lockTenantAuthority(ctx context.Context, tx *gorm.DB, actor types.MemberActorAuthority, tenantID uint64) (types.TenantRole, error) {
	var tenant types.Tenant
	if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id").Where("id = ?", tenantID).Take(&tenant).Error; err != nil {
		return "", err
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
	if actor.ServicePrincipal {
		return role != types.TenantRoleOwner && target != types.TenantRoleOwner
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

func activeOwnerCount(ctx context.Context, tx *gorm.DB, tenantID uint64) (int64, error) {
	var count int64
	err := tx.WithContext(ctx).Model(&types.TenantMember{}).Where(boundEnterpriseMembership).
		Where("tenant_id = ? AND role = ? AND status = ?", tenantID, types.TenantRoleOwner, types.TenantMemberStatusActive).
		Count(&count).Error
	return count, err
}

func (r *tenantMemberRepository) UpdateRole(ctx context.Context, actor types.MemberActorAuthority, userID string, tenantID uint64, role types.TenantRole) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		target, err := lockManagedMember(ctx, tx, actor, userID, tenantID, &role)
		if err != nil {
			return err
		}
		if target.Role == types.TenantRoleOwner && role != types.TenantRoleOwner && target.Status == types.TenantMemberStatusActive {
			owners, err := activeOwnerCount(ctx, tx, tenantID)
			if err != nil {
				return err
			}
			if owners <= 1 {
				return ErrLastOwner
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

// SoftDelete marks the membership row as deleted after the same tenant lock
// and current actor/target validation used by role and status changes.
func (r *tenantMemberRepository) SoftDelete(ctx context.Context, actor types.MemberActorAuthority, userID string, tenantID uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		target, err := lockManagedMember(ctx, tx, actor, userID, tenantID, nil)
		if err != nil {
			return err
		}
		if target.Role == types.TenantRoleOwner && target.Status == types.TenantMemberStatusActive {
			owners, err := activeOwnerCount(ctx, tx, tenantID)
			if err != nil {
				return err
			}
			if owners <= 1 {
				return ErrLastOwner
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

// CountActiveOwners reports the number of active owner rows in the tenant.
func (r *tenantMemberRepository) CountActiveOwners(ctx context.Context, tenantID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&types.TenantMember{}).
		Where(boundEnterpriseMembership).
		Where("tenant_id = ? AND role = ? AND status = ?",
			tenantID, types.TenantRoleOwner, types.TenantMemberStatusActive).
		Count(&count).Error
	return count, err
}

// TransferOwnership serializes on the tenant row, then swaps the roles in a
// single UPDATE. The explicit owner count makes historical corrupt state fail
// closed instead of selecting an arbitrary Owner.
func (r *tenantMemberRepository) TransferOwnership(
	ctx context.Context,
	actorUserID, targetUserID string,
	tenantID uint64,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant types.Tenant
		if err := tx.Clauses(forUpdateClause()).Select("id").Where("id = ?", tenantID).Take(&tenant).Error; err != nil {
			return err
		}

		var owners []types.TenantMember
		if err := tx.Clauses(forUpdateClause()).
			Where("tenant_id = ? AND role = ? AND status = ?", tenantID, types.TenantRoleOwner, types.TenantMemberStatusActive).
			Find(&owners).Error; err != nil {
			return err
		}
		if len(owners) != 1 {
			return ErrOwnershipInvariant
		}
		if owners[0].UserID != actorUserID {
			return ErrOwnershipTransferInvalid
		}
		if err := lockBoundEnterpriseUser(ctx, tx, owners[0].UserID, tenantID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOwnershipTransferInvalid
			}
			return err
		}

		var target types.TenantMember
		if err := tx.Clauses(forUpdateClause()).
			Where("user_id = ? AND tenant_id = ? AND role = ? AND status = ?", targetUserID, tenantID, types.TenantRoleAdmin, types.TenantMemberStatusActive).
			Take(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOwnershipTransferInvalid
			}
			return err
		}
		if err := lockBoundEnterpriseUser(ctx, tx, target.UserID, tenantID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOwnershipTransferInvalid
			}
			return err
		}

		// PostgreSQL's partial one-active-owner index checks every row update
		// immediately. Demote the verified current Owner first, then promote
		// the verified active Admin; a CASE update would transiently create two
		// owners and fail even though the final state is valid.
		res := tx.WithContext(ctx).Model(&types.TenantMember{}).
			Where("id = ? AND user_id = ? AND role = ? AND status = ?", owners[0].ID, actorUserID, types.TenantRoleOwner, types.TenantMemberStatusActive).
			Updates(map[string]any{"role": types.TenantRoleAdmin, "updated_at": time.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrOwnershipTransferInvalid
		}
		res = tx.WithContext(ctx).Model(&types.TenantMember{}).
			Where("id = ? AND user_id = ? AND role = ? AND status = ?", target.ID, targetUserID, types.TenantRoleAdmin, types.TenantMemberStatusActive).
			Updates(map[string]any{"role": types.TenantRoleOwner, "updated_at": time.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrOwnershipTransferInvalid
		}
		return nil
	})
}

// HasAnyMembers reports whether the tenant has at least one active
// membership row. Uses a LIMIT 1 SELECT (instead of COUNT(*)) so the query
// short-circuits after the first match — important because this is on the
// auth middleware's hot path for users without a cached membership.
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
