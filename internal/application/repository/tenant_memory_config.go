package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type tenantMemoryConfigRepository struct {
	db *gorm.DB
}

func NewTenantMemoryConfigRepository(db *gorm.DB) interfaces.TenantMemoryConfigRepository {
	return &tenantMemoryConfigRepository{db: db}
}

func normalizedTenantMemoryConfig(cfg *types.TenantMemoryConfig) *types.TenantMemoryConfig {
	if cfg == nil {
		cfg = &types.TenantMemoryConfig{}
	} else {
		copy := *cfg
		cfg = &copy
	}
	cfg.Normalize()
	return cfg
}

func normalizedMemoryRuntimeConfig(cfg *types.MemoryRuntimeConfig) *types.MemoryRuntimeConfig {
	if cfg == nil {
		return nil
	}
	copy := *cfg
	copy.Normalize()
	return &copy
}

func sameTenantMemoryConfig(a, b *types.TenantMemoryConfig) (bool, error) {
	left, err := json.Marshal(normalizedTenantMemoryConfig(a))
	if err != nil {
		return false, err
	}
	right, err := json.Marshal(normalizedTenantMemoryConfig(b))
	if err != nil {
		return false, err
	}
	return bytes.Equal(left, right), nil
}

func sameMemoryRuntimeConfig(a, b *types.MemoryRuntimeConfig) (bool, error) {
	left, err := json.Marshal(normalizedMemoryRuntimeConfig(a))
	if err != nil {
		return false, err
	}
	right, err := json.Marshal(normalizedMemoryRuntimeConfig(b))
	if err != nil {
		return false, err
	}
	return bytes.Equal(left, right), nil
}

func loadPlatformMemoryRuntimeConfig(
	ctx context.Context, tx *gorm.DB, lockStrength string,
) (*types.PlatformMemoryRuntimeConfig, error) {
	var row types.PlatformMemoryRuntimeConfig
	query := tx.WithContext(ctx).Where("id = ?", types.PlatformMemoryRuntimeConfigSingletonID)
	if lockStrength != "" {
		query = query.Clauses(clause.Locking{Strength: lockStrength})
	}
	if err := query.Take(&row).Error; err != nil {
		return nil, err
	}
	if row.Runtime == nil {
		return nil, fmt.Errorf("platform memory runtime singleton has no runtime configuration")
	}
	row.Runtime = normalizedMemoryRuntimeConfig(row.Runtime)
	if row.Generation < 0 {
		return nil, fmt.Errorf("platform memory runtime generation is negative")
	}
	return &row, nil
}

func effectiveMemoryGeneration(platform, tenant int64) (int64, error) {
	if platform < 0 || tenant < 0 {
		return 0, fmt.Errorf("memory generation is negative")
	}
	if platform > math.MaxInt64-tenant {
		return 0, fmt.Errorf("effective memory generation overflow")
	}
	return platform + tenant, nil
}

func tenantMemoryConfigState(
	runtime *types.PlatformMemoryRuntimeConfig, tenant *types.Tenant,
) (*interfaces.TenantMemoryConfigState, error) {
	if runtime == nil || tenant == nil {
		return nil, nil
	}
	consent := normalizedTenantMemoryConfig(tenant.MemoryConfig)
	generation, err := effectiveMemoryGeneration(runtime.Generation, tenant.MemoryGeneration)
	if err != nil {
		return nil, err
	}
	return &interfaces.TenantMemoryConfigState{
		Config: types.ComposeMemoryConfig(runtime.Runtime, consent), Consent: consent,
		Generation: generation, TenantGeneration: tenant.MemoryGeneration,
		PlatformGeneration: runtime.Generation,
	}, nil
}

func (r *tenantMemoryConfigRepository) Get(
	ctx context.Context, tenantID uint64,
) (*interfaces.TenantMemoryConfigState, error) {
	var result *interfaces.TenantMemoryConfigState
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		runtime, err := loadPlatformMemoryRuntimeConfig(ctx, tx, "SHARE")
		if err != nil {
			return err
		}
		var tenant types.Tenant
		err = tx.WithContext(ctx).Select("id", "memory_config", "memory_generation").
			Where("id = ?", tenantID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&tenant).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		result, err = tenantMemoryConfigState(runtime, &tenant)
		return err
	})
	return result, err
}

func (r *tenantMemoryConfigRepository) Update(
	ctx context.Context, tenantID uint64, cfg *types.TenantMemoryConfig,
) (*interfaces.TenantMemoryConfigState, error) {
	cfg = normalizedTenantMemoryConfig(cfg)
	var result *interfaces.TenantMemoryConfigState
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		runtime, err := loadPlatformMemoryRuntimeConfig(ctx, tx, "SHARE")
		if err != nil {
			return err
		}
		var tenant types.Tenant
		if err := tx.WithContext(ctx).Select("id", "memory_config", "memory_generation").
			Where("id = ?", tenantID).Clauses(forUpdateClause()).Take(&tenant).Error; err != nil {
			return err
		}
		same, err := sameTenantMemoryConfig(tenant.MemoryConfig, cfg)
		if err != nil {
			return err
		}
		if !same {
			tenant.MemoryGeneration++
			if err := tx.WithContext(ctx).Model(&types.Tenant{}).Where("id = ?", tenantID).
				Updates(map[string]interface{}{
					"memory_config": cfg, "memory_generation": tenant.MemoryGeneration,
					"updated_at": time.Now(),
				}).Error; err != nil {
				return err
			}
		}
		tenant.MemoryConfig = cfg
		result, err = tenantMemoryConfigState(runtime, &tenant)
		return err
	})
	return result, err
}
