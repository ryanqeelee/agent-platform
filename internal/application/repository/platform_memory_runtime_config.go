package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
)

// PlatformMemoryRuntimeConfigRepository persists the deployment singleton.
// Tenant scope is deliberately absent from this authority.
type PlatformMemoryRuntimeConfigRepository interface {
	Get(ctx context.Context) (*types.PlatformMemoryRuntimeConfig, error)
	Update(
		ctx context.Context, runtime *types.MemoryRuntimeConfig, actorID string, now time.Time,
		validate func(*gorm.DB) error,
	) (*types.PlatformMemoryRuntimeConfig, error)
}

type platformMemoryRuntimeConfigRepository struct{ db *gorm.DB }

func NewPlatformMemoryRuntimeConfigRepository(db *gorm.DB) PlatformMemoryRuntimeConfigRepository {
	return &platformMemoryRuntimeConfigRepository{db: db}
}

func (r *platformMemoryRuntimeConfigRepository) Get(
	ctx context.Context,
) (*types.PlatformMemoryRuntimeConfig, error) {
	var row types.PlatformMemoryRuntimeConfig
	err := r.db.WithContext(ctx).Where("id = ?", types.PlatformMemoryRuntimeConfigSingletonID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if row.Runtime == nil {
		return nil, errors.New("platform memory runtime singleton has no runtime configuration")
	}
	row.Runtime = normalizedMemoryRuntimeConfig(row.Runtime)
	return &row, nil
}

func (r *platformMemoryRuntimeConfigRepository) Update(
	ctx context.Context,
	runtime *types.MemoryRuntimeConfig,
	actorID string,
	now time.Time,
	validate func(*gorm.DB) error,
) (*types.PlatformMemoryRuntimeConfig, error) {
	if runtime == nil {
		return nil, errors.New("platform memory runtime configuration is required")
	}
	runtime = normalizedMemoryRuntimeConfig(runtime)
	var result *types.PlatformMemoryRuntimeConfig
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if validate != nil {
			if err := validate(tx); err != nil {
				return err
			}
		}
		row, err := loadPlatformMemoryRuntimeConfig(ctx, tx, "UPDATE")
		if err != nil {
			return err
		}
		same, err := sameMemoryRuntimeConfig(row.Runtime, runtime)
		if err != nil {
			return err
		}
		if same {
			result = row
			return nil
		}
		row.Generation++
		if err := tx.WithContext(ctx).Model(&types.PlatformMemoryRuntimeConfig{}).
			Where("id = ?", types.PlatformMemoryRuntimeConfigSingletonID).
			Updates(map[string]interface{}{
				"runtime": runtime, "generation": row.Generation,
				"updated_by": actorID, "updated_at": now,
			}).Error; err != nil {
			return err
		}
		row.Runtime = runtime
		row.UpdatedBy = actorID
		row.UpdatedAt = now
		result = row
		return nil
	})
	return result, err
}
