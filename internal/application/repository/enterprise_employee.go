package repository

import (
	"context"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// CreateEmployee commits the new identity and membership together. The same tenant
// lock used by membership revocation rechecks administrator authority before writing.
func (r *tenantMemberRepository) CreateEmployee(ctx context.Context, actor types.MemberActorAuthority, user *types.User, member *types.TenantMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		role, err := lockTenantAuthority(ctx, tx, actor, user.TenantID)
		if err != nil {
			return err
		}
		if actor.ServicePrincipal || role != types.TenantRoleAdmin {
			return ErrMemberActionForbidden
		}
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return createTenantMember(ctx, tx, member)
	})
}
