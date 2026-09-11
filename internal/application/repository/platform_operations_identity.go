package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrPlatformOperationConflict        = errors.New("repository: platform operation conflicts with durable receipt")
	ErrPlatformOperationNotFound        = errors.New("repository: platform operation receipt not found")
	ErrPlatformOperationTargetForbidden = errors.New("repository: platform operation target is forbidden")
)

type platformOperationsIdentityRepository struct{ db *gorm.DB }

func NewPlatformOperationsIdentityRepository(db *gorm.DB) interfaces.PlatformOperationsIdentityRepository {
	return &platformOperationsIdentityRepository{db: db}
}

func lockPlatformSystemAdministrator(ctx context.Context, tx *gorm.DB, actorUserID string) error {
	if actorUserID == "" {
		return ErrMemberActionForbidden
	}
	var actor types.User
	err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id").
		Where("id = ? AND is_active = ? AND is_system_admin = ? AND (tenant_id IS NULL OR tenant_id = 0) AND can_access_all_tenants = ?",
			actorUserID, true, true, false).
		Take(&actor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMemberActionForbidden
	}
	return err
}

func (r *platformOperationsIdentityRepository) CreateInitialAdministrator(
	ctx context.Context,
	actorUserID, commandID, requestSHA256, password string,
	user *types.User,
) (*types.PlatformInitialAdministratorReceipt, *types.User, bool, error) {
	var receipt *types.PlatformInitialAdministratorReceipt
	var result *types.User
	replayed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformSystemAdministrator(ctx, tx, actorUserID); err != nil {
			return err
		}
		var existingReceipt types.PlatformInitialAdministratorReceipt
		err := tx.WithContext(ctx).Clauses(forUpdateClause()).Where("command_id = ?", commandID).Take(&existingReceipt).Error
		if err == nil {
			if existingReceipt.RequestSHA256 != requestSHA256 || existingReceipt.Username != user.Username || existingReceipt.Email != user.Email {
				return ErrPlatformOperationConflict
			}
			var existingUser types.User
			if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Where("id = ?", existingReceipt.UserID).Take(&existingUser).Error; err != nil {
				return ErrPlatformOperationConflict
			}
			if !existingUser.IsActive || existingUser.IsSystemAdmin || existingUser.CanAccessAllTenants ||
				types.IsSyntheticUserID(existingUser.ID) || existingUser.Username != user.Username || existingUser.Email != user.Email ||
				bcrypt.CompareHashAndPassword([]byte(existingUser.PasswordHash), []byte(password)) != nil {
				return ErrPlatformOperationConflict
			}
			receipt, result, replayed = &existingReceipt, &existingUser, true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var collision int64
		if err := tx.WithContext(ctx).Unscoped().Model(&types.User{}).
			Where("id = ? OR username = ? OR email = ?", user.ID, user.Username, user.Email).Count(&collision).Error; err != nil {
			return err
		}
		if collision != 0 {
			return ErrPlatformOperationConflict
		}
		if err := tx.WithContext(ctx).Omit("tenant_id").Create(user).Error; err != nil {
			return err
		}
		now := time.Now()
		created := &types.PlatformInitialAdministratorReceipt{
			CommandID: commandID, RequestSHA256: requestSHA256, UserID: user.ID,
			Username: user.Username, Email: user.Email, Status: "created", CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.WithContext(ctx).Create(created).Error; err != nil {
			return err
		}
		receipt, result = created, user
		return nil
	})
	return receipt, result, replayed, err
}

func (r *platformOperationsIdentityRepository) GetInitialAdministrator(
	ctx context.Context, actorUserID, commandID string,
) (*types.PlatformInitialAdministratorReceipt, *types.User, error) {
	var receipt types.PlatformInitialAdministratorReceipt
	var user types.User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockPlatformSystemAdministrator(ctx, tx, actorUserID); err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Where("command_id = ?", commandID).Take(&receipt).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPlatformOperationNotFound
			}
			return err
		}
		if err := tx.WithContext(ctx).Where("id = ?", receipt.UserID).Take(&user).Error; err != nil {
			return ErrPlatformOperationConflict
		}
		if user.IsSystemAdmin || user.CanAccessAllTenants || types.IsSyntheticUserID(user.ID) ||
			user.Username != receipt.Username || user.Email != receipt.Email {
			return ErrPlatformOperationConflict
		}
		return nil
	})
	return &receipt, &user, err
}

func (r *platformOperationsIdentityRepository) ResetEnterpriseMemberPassword(
	ctx context.Context,
	actorUserID string,
	tenantID uint64,
	targetUserID, passwordHash string,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant types.Tenant
		if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id", "status").Where("id = ?", tenantID).Take(&tenant).Error; err != nil {
			return err
		}
		if tenant.Status != types.TenantStatusActive {
			return ErrEnterpriseNotActive
		}
		if err := lockPlatformSystemAdministrator(ctx, tx, actorUserID); err != nil {
			return err
		}
		if actorUserID == targetUserID || isSpecialEnterpriseUserID(targetUserID) {
			return ErrPlatformOperationTargetForbidden
		}
		var member types.TenantMember
		if err := tx.WithContext(ctx).Clauses(forUpdateClause()).Select("id").
			Where("user_id = ? AND tenant_id = ? AND status = ?",
				targetUserID, tenantID, types.TenantMemberStatusActive).Take(&member).Error; err != nil {
			return ErrPlatformOperationTargetForbidden
		}
		var target types.User
		if err := tx.WithContext(ctx).Clauses(forUpdateClause()).
			Where("id = ? AND tenant_id = ? AND is_active = ? AND is_system_admin = ? AND can_access_all_tenants = ?",
				targetUserID, tenantID, true, false, false).Take(&target).Error; err != nil {
			return ErrPlatformOperationTargetForbidden
		}
		now := time.Now()
		updated := tx.WithContext(ctx).Model(&types.User{}).Where("id = ? AND tenant_id = ?", target.ID, tenantID).
			Updates(map[string]any{"password_hash": passwordHash, "updated_at": now})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrPlatformOperationTargetForbidden
		}
		return tx.WithContext(ctx).Model(&types.AuthToken{}).Where("user_id = ? AND is_revoked = ?", target.ID, false).
			Updates(map[string]any{"is_revoked": true, "updated_at": now}).Error
	})
}
