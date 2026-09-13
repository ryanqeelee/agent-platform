package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// TenantSandboxPermissionService owns enterprise sandbox permissions. Shared
// provider configuration is deliberately not reachable through this service.
type TenantSandboxPermissionService struct {
	db *gorm.DB
}

func NewTenantSandboxPermissionService(db *gorm.DB) *TenantSandboxPermissionService {
	return &TenantSandboxPermissionService{db: db}
}

func (s *TenantSandboxPermissionService) WorkspaceScriptsDisabled(
	ctx context.Context, tenantID uint64,
) (bool, error) {
	if tenantID == 0 {
		return false, errors.New("sandbox permission requires a positive tenant ID")
	}
	var tenant types.Tenant
	err := s.db.WithContext(ctx).Select("id", "sandbox_scripts_disabled").
		Where("id = ?", tenantID).Take(&tenant).Error
	if err != nil {
		return false, fmt.Errorf("read tenant sandbox permission: %w", err)
	}
	return tenant.SandboxScriptsDisabled, nil
}

func (s *TenantSandboxPermissionService) SetWorkspaceScriptsDisabled(
	ctx context.Context, tenantID uint64, disabled bool,
) error {
	if tenantID == 0 {
		return errors.New("sandbox permission requires a positive tenant ID")
	}
	result := s.db.WithContext(ctx).Model(&types.Tenant{}).
		Where("id = ?", tenantID).
		Update("sandbox_scripts_disabled", disabled)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
