package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

var ErrEnterpriseNotActive = errors.New("repository: enterprise is not active")

// CreateEmployee commits the new identity and membership together. The same tenant
// lock used by membership revocation rechecks administrator authority before writing.
func (r *tenantMemberRepository) CreateEmployee(ctx context.Context, actor types.MemberActorAuthority, user *types.User, member *types.TenantMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant types.Tenant
		if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id", "status").
			Where("id = ?", user.TenantID).Take(&tenant).Error; err != nil {
			return err
		}
		if tenant.Status != types.TenantStatusActive {
			return ErrEnterpriseNotActive
		}
		role, err := lockTenantAuthority(ctx, tx, actor, user.TenantID)
		if err != nil {
			return err
		}
		if actor.ServicePrincipal || (!actor.SystemAdministrator && role != types.TenantRoleAdmin) {
			return ErrMemberActionForbidden
		}
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return createTenantMember(ctx, tx, member)
	})
}
