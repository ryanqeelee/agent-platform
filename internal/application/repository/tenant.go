package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrTenantNotFound                      = errors.New("tenant not found")
	ErrTenantHasKnowledgeBase              = errors.New("tenant has associated knowledge bases")
	ErrEnterpriseActivationConflict        = errors.New("repository: enterprise activation conflict")
	ErrEnterpriseActivationStorageRequired = errors.New("repository: enterprise activation default storage required")
)

// tenantRepository implements tenant repository interface
type tenantRepository struct {
	db *gorm.DB
}

// NewTenantRepository creates a new tenant repository
func NewTenantRepository(db *gorm.DB) interfaces.TenantRepository {
	return &tenantRepository{db: db}
}

func enterpriseActivationConflict(reason string) error {
	return fmt.Errorf("%w: %s", ErrEnterpriseActivationConflict, reason)
}

func isOrdinaryActivationOwner(user *types.User) bool {
	if user == nil || !user.IsActive || user.IsSystemAdmin || user.CanAccessAllTenants ||
		types.IsSyntheticUserID(user.ID) {
		return false
	}
	for _, prefix := range []string{
		types.PrincipalAPITenant + ":",
		types.PrincipalAPIPlatform + ":",
		types.PrincipalAPIExternalUser + ":",
	} {
		if strings.HasPrefix(user.ID, prefix) {
			return false
		}
	}
	return true
}

func activationStateFromTenantStatus(status string) (types.EnterpriseActivationState, bool) {
	switch status {
	case types.TenantStatusProvisioning:
		return types.EnterpriseActivationStatePrepared, true
	case types.TenantStatusActive:
		return types.EnterpriseActivationStateActive, true
	case types.TenantStatusActivationAbandoned:
		return types.EnterpriseActivationStateAbandoned, true
	default:
		return "", false
	}
}

func loadActivationTenant(
	ctx context.Context,
	tx *gorm.DB,
	activationID string,
) (*types.Tenant, error) {
	var tenant types.Tenant
	err := tx.WithContext(ctx).Unscoped().Clauses(forUpdateClause()).
		Where("ringxun_activation_id = ?", activationID).Take(&tenant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &tenant, nil
}

func applyExistingActivation(
	ctx context.Context,
	tx *gorm.DB,
	command interfaces.EnterpriseActivationCommand,
	tenant *types.Tenant,
) (*interfaces.EnterpriseActivationResult, error) {
	// The lookup predicate already proves this row owns command.ActivationID.
	if tenant.DeletedAt.Valid || tenant.RingxunActivationRequestSHA256 == nil ||
		*tenant.RingxunActivationRequestSHA256 != command.RequestSHA256 ||
		tenant.RingxunInitialOwnerUserID == nil ||
		*tenant.RingxunInitialOwnerUserID != command.FirstOwnerUserID ||
		tenant.Name != command.TenantName || tenant.Description != command.TenantDescription {
		return nil, enterpriseActivationConflict("activation receipt does not match the request")
	}

	state, ok := activationStateFromTenantStatus(tenant.Status)
	if !ok {
		return nil, enterpriseActivationConflict("activation receipt has an invalid tenant status")
	}

	var user types.User
	if err := tx.WithContext(ctx).Clauses(forUpdateClause()).
		Where("id = ?", command.FirstOwnerUserID).Take(&user).Error; err != nil {
		return nil, enterpriseActivationConflict("initial owner is unavailable")
	}
	if !isOrdinaryActivationOwner(&user) || user.TenantID != tenant.ID {
		return nil, enterpriseActivationConflict("initial owner no longer matches the activation tenant")
	}

	var member types.TenantMember
	if err := tx.WithContext(ctx).Clauses(forUpdateClause()).
		Where("user_id = ? AND tenant_id = ?", command.FirstOwnerUserID, tenant.ID).
		Take(&member).Error; err != nil {
		return nil, enterpriseActivationConflict("initial owner membership is unavailable")
	}
	if member.Role != types.TenantRoleAdmin {
		return nil, enterpriseActivationConflict("initial owner membership is not owner")
	}
	expectedMemberStatus := types.TenantMemberStatusSuspended
	if state == types.EnterpriseActivationStateActive {
		expectedMemberStatus = types.TenantMemberStatusActive
	}
	if member.Status != expectedMemberStatus {
		return nil, enterpriseActivationConflict("tenant and owner membership states disagree")
	}
	current := &interfaces.EnterpriseActivationResult{
		ActivationID:      command.ActivationID,
		TenantID:          tenant.ID,
		OwnerMembershipID: member.ID,
		RequestSHA256:     command.RequestSHA256,
		State:             state,
	}

	now := time.Now()
	switch command.DesiredState {
	case types.EnterpriseActivationStatePrepared:
		// Prepared is recognition-only once the receipt has advanced. Never
		// regress active or terminal state on an older exact replay.
	case types.EnterpriseActivationStateActive:
		switch state {
		case types.EnterpriseActivationStateActive:
			// Exact replay.
		case types.EnterpriseActivationStateAbandoned:
			return nil, enterpriseActivationConflict("abandoned activation cannot become active")
		case types.EnterpriseActivationStatePrepared:
			if tenant.DefaultStorageBackendID == nil || strings.TrimSpace(*tenant.DefaultStorageBackendID) == "" {
				return current, ErrEnterpriseActivationStorageRequired
			}
			memberUpdate := tx.WithContext(ctx).Model(&types.TenantMember{}).
				Where("id = ? AND role = ? AND status = ?", member.ID, types.TenantRoleAdmin, types.TenantMemberStatusSuspended).
				Updates(map[string]any{
					"status":     types.TenantMemberStatusActive,
					"joined_at":  now,
					"updated_at": now,
				})
			if memberUpdate.Error != nil {
				return nil, memberUpdate.Error
			}
			if memberUpdate.RowsAffected != 1 {
				return nil, enterpriseActivationConflict("initial owner membership changed concurrently")
			}
			tenantUpdate := tx.WithContext(ctx).Model(&types.Tenant{}).
				Where("id = ? AND status = ?", tenant.ID, types.TenantStatusProvisioning).
				Updates(map[string]any{"status": types.TenantStatusActive, "updated_at": now})
			if tenantUpdate.Error != nil {
				return nil, tenantUpdate.Error
			}
			if tenantUpdate.RowsAffected != 1 {
				return nil, enterpriseActivationConflict("activation tenant changed concurrently")
			}
			state = types.EnterpriseActivationStateActive
		}
	case types.EnterpriseActivationStateAbandoned:
		switch state {
		case types.EnterpriseActivationStateAbandoned:
			// Exact replay.
		case types.EnterpriseActivationStateActive:
			return nil, enterpriseActivationConflict("active activation cannot be abandoned")
		case types.EnterpriseActivationStatePrepared:
			tenantUpdate := tx.WithContext(ctx).Model(&types.Tenant{}).
				Where("id = ? AND status = ?", tenant.ID, types.TenantStatusProvisioning).
				Updates(map[string]any{"status": types.TenantStatusActivationAbandoned, "updated_at": now})
			if tenantUpdate.Error != nil {
				return nil, tenantUpdate.Error
			}
			if tenantUpdate.RowsAffected != 1 {
				return nil, enterpriseActivationConflict("activation tenant changed concurrently")
			}
			state = types.EnterpriseActivationStateAbandoned
		}
	default:
		return nil, enterpriseActivationConflict("invalid desired activation state")
	}

	current.State = state
	return current, nil
}

// ApplyEnterpriseActivation atomically creates or advances the one tenant and
// initial Owner membership named by a Ringxun activation receipt.
func (r *tenantRepository) ApplyEnterpriseActivation(
	ctx context.Context,
	command interfaces.EnterpriseActivationCommand,
) (*interfaces.EnterpriseActivationResult, error) {
	var result *interfaces.EnterpriseActivationResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tenant, err := loadActivationTenant(ctx, tx, command.ActivationID)
		if err != nil {
			return err
		}
		if tenant != nil {
			result, err = applyExistingActivation(ctx, tx, command, tenant)
			return err
		}
		if command.DesiredState != types.EnterpriseActivationStatePrepared {
			return enterpriseActivationConflict("activation must be prepared before it can advance")
		}

		var user types.User
		if err := tx.WithContext(ctx).Clauses(forUpdateClause()).
			Where("id = ?", command.FirstOwnerUserID).Take(&user).Error; err != nil {
			return enterpriseActivationConflict("initial owner is unavailable")
		}

		// A same-owner concurrent request may have committed while this call
		// waited for the user lock. Re-read the globally unique receipt before
		// applying tenantless-user preconditions.
		tenant, err = loadActivationTenant(ctx, tx, command.ActivationID)
		if err != nil {
			return err
		}
		if tenant != nil {
			result, err = applyExistingActivation(ctx, tx, command, tenant)
			return err
		}
		if !isOrdinaryActivationOwner(&user) || user.TenantID != 0 {
			return enterpriseActivationConflict("initial owner must be an active tenantless ordinary user")
		}
		var existingMemberships int64
		if err := tx.WithContext(ctx).Model(&types.TenantMember{}).
			Where("user_id = ?", user.ID).Count(&existingMemberships).Error; err != nil {
			return err
		}
		if existingMemberships != 0 {
			return enterpriseActivationConflict("initial owner already has a membership")
		}

		activationID := command.ActivationID
		requestSHA := command.RequestSHA256
		ownerID := command.FirstOwnerUserID
		tenant = &types.Tenant{
			Name:                           command.TenantName,
			Description:                    command.TenantDescription,
			Status:                         types.TenantStatusProvisioning,
			RingxunActivationID:            &activationID,
			RingxunActivationRequestSHA256: &requestSHA,
			RingxunInitialOwnerUserID:      &ownerID,
		}
		created := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(tenant)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			// A different-owner concurrent caller may own the receipt. ON
			// CONFLICT keeps this transaction usable so we can compare it.
			tenant, err = loadActivationTenant(ctx, tx, command.ActivationID)
			if err != nil {
				return err
			}
			if tenant == nil {
				return enterpriseActivationConflict("activation receipt was claimed concurrently")
			}
			result, err = applyExistingActivation(ctx, tx, command, tenant)
			return err
		}

		bound := tx.WithContext(ctx).Model(&types.User{}).
			Where("id = ? AND (tenant_id IS NULL OR tenant_id = ?) AND is_active = ? AND is_system_admin = ? AND can_access_all_tenants = ?",
				user.ID, 0, true, false, false).
			Update("tenant_id", tenant.ID)
		if bound.Error != nil {
			return bound.Error
		}
		if bound.RowsAffected != 1 {
			return enterpriseActivationConflict("initial owner changed concurrently")
		}

		now := time.Now()
		member := &types.TenantMember{
			UserID:    user.ID,
			TenantID:  tenant.ID,
			Role:      types.TenantRoleAdmin,
			Status:    types.TenantMemberStatusSuspended,
			JoinedAt:  now,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := tx.WithContext(ctx).Create(member).Error; err != nil {
			return err
		}
		result = &interfaces.EnterpriseActivationResult{
			ActivationID:      command.ActivationID,
			TenantID:          tenant.ID,
			OwnerMembershipID: member.ID,
			RequestSHA256:     command.RequestSHA256,
			State:             types.EnterpriseActivationStatePrepared,
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

// CreateTenant creates tenant
func (r *tenantRepository) CreateTenant(ctx context.Context, tenant *types.Tenant) error {
	return r.db.WithContext(ctx).Create(tenant).Error
}

// GetTenantByID gets tenant by ID
func (r *tenantRepository) GetTenantByID(ctx context.Context, id uint64) (*types.Tenant, error) {
	var tenant types.Tenant
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTenantNotFound
		}
		return nil, err
	}
	return &tenant, nil
}

// GetTenantsByIDs batches GetTenantByID with a single IN-list query.
// Returns a map keyed by tenant ID; missing rows are simply absent from
// the map (no error). An empty input slice short-circuits to an empty map
// without hitting the database.
func (r *tenantRepository) GetTenantsByIDs(ctx context.Context, ids []uint64) (map[uint64]*types.Tenant, error) {
	if len(ids) == 0 {
		return map[uint64]*types.Tenant{}, nil
	}
	var tenants []*types.Tenant
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&tenants).Error; err != nil {
		return nil, err
	}
	out := make(map[uint64]*types.Tenant, len(tenants))
	for _, t := range tenants {
		if t != nil {
			out[t.ID] = t
		}
	}
	return out, nil
}

// ListTenants lists all tenants
func (r *tenantRepository) ListTenants(ctx context.Context) ([]*types.Tenant, error) {
	var tenants []*types.Tenant
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&tenants).Error; err != nil {
		return nil, err
	}
	return tenants, nil
}

// SearchTenants searches tenants with pagination and filters
func (r *tenantRepository) SearchTenants(ctx context.Context, keyword string, tenantID uint64, page, pageSize int) ([]*types.Tenant, int64, error) {
	var tenants []*types.Tenant
	var total int64

	query := r.db.WithContext(ctx).Model(&types.Tenant{})

	// Build search conditions
	if tenantID > 0 && keyword != "" {
		escaped := escapeLikeKeyword(keyword)
		query = query.Where("id = ? OR name LIKE ? OR description LIKE ?", tenantID, "%"+escaped+"%", "%"+escaped+"%")
	} else if tenantID > 0 {
		query = query.Where("id = ?", tenantID)
	} else if keyword != "" {
		escaped := escapeLikeKeyword(keyword)
		query = query.Where("name LIKE ? OR description LIKE ?", "%"+escaped+"%", "%"+escaped+"%")
	}

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if page > 0 && pageSize > 0 {
		offset := (page - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	// Order by created_at DESC
	query = query.Order("created_at DESC")

	// Execute query
	if err := query.Find(&tenants).Error; err != nil {
		return nil, 0, err
	}

	return tenants, total, nil
}

// UpdateTenant updates tenant.
func (r *tenantRepository) UpdateTenant(ctx context.Context, tenant *types.Tenant) error {
	return r.db.WithContext(ctx).Model(&types.Tenant{}).Where("id = ?", tenant.ID).Updates(tenant).Error
}

func (r *tenantRepository) SetDefaultStorageBackend(ctx context.Context, tenantID uint64, backendID string) error {
	return r.db.WithContext(ctx).Model(&types.Tenant{}).Where("id = ?", tenantID).
		Update("default_storage_backend_id", backendID).Error
}

// DeleteTenant soft-deletes the tenant and every active membership row
// for that tenant in one transaction. Without the membership purge,
// /auth/me still lists the defunct tenant (name lookup fails → UI shows
// "#<id>").
func (r *tenantRepository) DeleteTenant(ctx context.Context, id uint64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("tenant_id = ?", id).Delete(&types.TenantMember{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&types.Tenant{}).Error
	})
}

func (r *tenantRepository) AdjustStorageUsed(ctx context.Context, tenantID uint64, delta int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant types.Tenant
		// 使用悲观锁确保并发安全
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&tenant, tenantID).Error; err != nil {
			return err
		}

		tenant.StorageUsed += delta
		// 保存更新并验证业务规则
		if tenant.StorageUsed < 0 {
			logger.Errorf(ctx, "tenant storage used is negative %d: %d", tenant.ID, tenant.StorageUsed)
			tenant.StorageUsed = 0
		}

		return tx.Save(&tenant).Error
	})
}

// BulkSetStorageQuota writes quotaBytes to storage_quota for every
// tenant in one statement. We don't WHERE-filter (the action is
// "apply globally"), so the affected count equals the row count of
// the tenants table.
//
// No transaction here: the operation is a single statement and we
// don't want to hold a long lock just to update a single column. If
// a concurrent CreateTenant lands in the middle, the new row gets
// the new default via the system-setting resolver in the handler —
// no risk of the new tenant being skipped.
func (r *tenantRepository) BulkSetStorageQuota(ctx context.Context, quotaBytes int64) (int64, error) {
	res := r.db.WithContext(ctx).
		Model(&types.Tenant{}).
		Where("1 = 1"). // GORM refuses unconditional UPDATEs without an explicit WHERE
		Update("storage_quota", quotaBytes)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}
