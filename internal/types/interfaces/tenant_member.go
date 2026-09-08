package interfaces

import (
	"context"

	"github.com/Tencent/WeKnora/internal/types"
)

// TenantMemberRepository persists (user, tenant) membership rows that
// carry the per-tenant TenantRole.
//
// All methods operate on active rows only (deleted_at IS NULL) unless the
// docstring explicitly says otherwise. Soft deletion is handled by GORM
// via the DeletedAt field on TenantMember.
type TenantMemberRepository interface {
	CreateEmployee(ctx context.Context, actor types.MemberActorAuthority, user *types.User, member *types.TenantMember) error
	// Create atomically binds a tenantless user to the target enterprise and
	// inserts the active membership. A user already bound elsewhere is rejected.
	Create(ctx context.Context, member *types.TenantMember) error

	// Get returns the non-deleted membership for the given (user, tenant),
	// including suspended rows so auth can distinguish a revocation from an
	// absent bootstrap row. It returns (nil, nil) when no row exists.
	Get(ctx context.Context, userID string, tenantID uint64) (*types.TenantMember, error)

	// ListByUser returns every active membership owned by the given user,
	// ordered by joined_at ascending.
	ListByUser(ctx context.Context, userID string) ([]*types.TenantMember, error)

	// ListByTenant returns every active membership inside the given tenant,
	// ordered by joined_at ascending.
	ListByTenant(ctx context.Context, tenantID uint64) ([]*types.TenantMember, error)

	// CountFilteredByTenant counts active memberships in tenant. Optional
	// search matches user email/username (join users table); empty search
	// counts all memberships.
	CountFilteredByTenant(ctx context.Context, tenantID uint64, search string) (int64, error)

	// ListPagedByTenant returns active memberships sorted joined_at ASC, id ASC.
	// search filters by user email/username (join users table); empty
	// search lists all memberships in tenant.
	ListPagedByTenant(ctx context.Context, tenantID uint64, search string, offset, limit int) ([]*types.TenantMember, error)

	// UpdateRole changes the role of an existing active membership. Returns
	// gorm.ErrRecordNotFound if no active row matches.
	UpdateRole(ctx context.Context, actor types.MemberActorAuthority, userID string, tenantID uint64, role types.TenantRole) error

	// UpdateStatus transitions an existing non-deleted membership between
	// active and suspended without changing its role.
	UpdateStatus(ctx context.Context, actor types.MemberActorAuthority, userID string, tenantID uint64, status types.TenantMemberStatus) error

	// UpdateOperatingAnalysisAccess changes the explicit analysis permission.
	// It returns false for an idempotent replay that made no change.
	UpdateOperatingAnalysisAccess(ctx context.Context, actor types.MemberActorAuthority, userID string, tenantID uint64, enabled bool) (bool, error)

	// SoftDelete marks the active membership as deleted. The user record
	// itself is untouched.
	SoftDelete(ctx context.Context, actor types.MemberActorAuthority, userID string, tenantID uint64) error

	// CreateManaged locks the tenant and resolves current actor authority
	// before adding a member.
	CreateManaged(ctx context.Context, actor types.MemberActorAuthority, member *types.TenantMember) error

	// CountActiveAdministrators reports how many active rows in the tenant carry
	// the admin role. Used by service-layer invariant checks ("cannot
	// remove the last administrator").
	CountActiveAdministrators(ctx context.Context, tenantID uint64) (int64, error)

	// HasAnyMembers reports whether the tenant has at least one active
	// membership. Used by the auth middleware to decide whether to
	// auto-promote the first authenticating human in an API-key-only tenant.
	HasAnyMembers(ctx context.Context, tenantID uint64) (bool, error)
}
