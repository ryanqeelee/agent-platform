// Package repository persists platform sandbox backend configuration.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const sandboxDefaultAdvisoryLockKey int64 = 0x574B4E53424F58

var ErrDeleteDefaultSandboxConfig = errors.New("default sandbox config cannot be deleted")

// TenantSandboxConfigRepository persists platform-owned named sandbox backend
// configs. Its methods intentionally accept no tenant identifier.
type TenantSandboxConfigRepository interface {
	Create(ctx context.Context, e *types.TenantSandboxConfigEntity) error
	GetByID(ctx context.Context, id string) (*types.TenantSandboxConfigEntity, error)
	GetDefault(ctx context.Context) (*types.TenantSandboxConfigEntity, error)
	SetDefault(ctx context.Context, id string) error
	ListAll(ctx context.Context) ([]*types.TenantSandboxConfigEntity, error)
	Update(ctx context.Context, e *types.TenantSandboxConfigEntity) error
	SoftDelete(ctx context.Context, id string) error
	SetCordon(ctx context.Context, id string, at time.Time) error
	ClearCordon(ctx context.Context, id string) error
}

type tenantSandboxConfigRepository struct {
	db *gorm.DB
}

// NewTenantSandboxConfigRepository returns a GORM-backed implementation.
func NewTenantSandboxConfigRepository(db *gorm.DB) TenantSandboxConfigRepository {
	return &tenantSandboxConfigRepository{db: db}
}

func (r *tenantSandboxConfigRepository) Create(
	ctx context.Context, e *types.TenantSandboxConfigEntity,
) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *tenantSandboxConfigRepository) GetByID(
	ctx context.Context, id string,
) (*types.TenantSandboxConfigEntity, error) {
	var e types.TenantSandboxConfigEntity
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *tenantSandboxConfigRepository) GetDefault(
	ctx context.Context,
) (*types.TenantSandboxConfigEntity, error) {
	var e types.TenantSandboxConfigEntity
	err := r.db.WithContext(ctx).Where("is_default = ?", true).First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func lockSandboxDefaultDomain(tx *gorm.DB) error {
	if tx.Dialector.Name() == "postgres" {
		return tx.Exec("SELECT pg_advisory_xact_lock(?)", sandboxDefaultAdvisoryLockKey).Error
	}
	// SQLite serializes writers. A no-op update acquires its database write
	// lock before either the current default or selected row is inspected.
	if tx.Dialector.Name() == "sqlite" {
		return tx.Exec("UPDATE platform_sandbox_configs SET is_default = is_default WHERE is_default = ?", true).Error
	}
	return nil
}

func (r *tenantSandboxConfigRepository) SetDefault(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSandboxDefaultDomain(tx); err != nil {
			return err
		}
		if id != "" {
			var selected types.TenantSandboxConfigEntity
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", id).Take(&selected).Error; err != nil {
				return fmt.Errorf("select sandbox default %q: %w", id, err)
			}
		}
		if err := tx.Model(&types.TenantSandboxConfigEntity{}).
			Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return err
		}
		if id == "" {
			return nil
		}
		result := tx.Model(&types.TenantSandboxConfigEntity{}).
			Where("id = ?", id).Update("is_default", true)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *tenantSandboxConfigRepository) ListAll(
	ctx context.Context,
) ([]*types.TenantSandboxConfigEntity, error) {
	var list []*types.TenantSandboxConfigEntity
	err := r.db.WithContext(ctx).
		Order("created_at ASC, id ASC").
		Find(&list).Error
	if err != nil {
		return nil, err
	}
	return list, nil
}

// Update writes the mutable columns. Select is explicit so a zero-valued
// CordonedAt on the passed entity cannot silently release someone else's lease.
func (r *tenantSandboxConfigRepository) Update(
	ctx context.Context, e *types.TenantSandboxConfigEntity,
) error {
	return r.db.WithContext(ctx).
		Model(&types.TenantSandboxConfigEntity{}).
		Where("id = ?", e.ID).
		Select("name", "description", "sandbox_type", "config", "updated_at").
		Updates(map[string]any{
			"name":         e.Name,
			"description":  e.Description,
			"sandbox_type": e.SandboxType,
			"config":       e.Config,
			"updated_at":   time.Now(),
		}).Error
}

func (r *tenantSandboxConfigRepository) SoftDelete(
	ctx context.Context, id string,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSandboxDefaultDomain(tx); err != nil {
			return err
		}
		var selected types.TenantSandboxConfigEntity
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).Take(&selected).Error; err != nil {
			return err
		}
		if selected.IsDefault {
			return ErrDeleteDefaultSandboxConfig
		}
		return tx.Delete(&selected).Error
	})
}

// ErrSandboxConfigCordoned is returned by SetCordon when another request
// already holds a fresh cordon lease on the same config row.
var ErrSandboxConfigCordoned = errors.New("sandbox config is being modified by another request")

// SetCordon must be committed before the caller lists provider sandboxes:
// resolution paths only stop creating sandboxes once they can see it.
//
// The update is a conditional CAS: it refuses to overwrite a cordon that is
// still within the lease window, so two concurrent identity-change requests
// cannot race past each other.
func (r *tenantSandboxConfigRepository) SetCordon(
	ctx context.Context, id string, at time.Time,
) error {
	result := r.db.WithContext(ctx).
		Model(&types.TenantSandboxConfigEntity{}).
		Where("id = ?", id).
		Where("cordoned_at IS NULL OR cordoned_at < ?", at.Add(-types.SandboxCordonLease)).
		Update("cordoned_at", at)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSandboxConfigCordoned
	}
	return nil
}

func (r *tenantSandboxConfigRepository) ClearCordon(
	ctx context.Context, id string,
) error {
	return r.db.WithContext(ctx).
		Model(&types.TenantSandboxConfigEntity{}).
		Where("id = ?", id).
		Update("cordoned_at", nil).Error
}
