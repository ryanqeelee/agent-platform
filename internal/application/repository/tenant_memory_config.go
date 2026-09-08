package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type tenantMemoryConfigRepository struct {
	db *gorm.DB
}

func NewTenantMemoryConfigRepository(db *gorm.DB) interfaces.TenantMemoryConfigRepository {
	return &tenantMemoryConfigRepository{db: db}
}

func (r *tenantMemoryConfigRepository) Get(
	ctx context.Context, tenantID uint64,
) (*interfaces.TenantMemoryConfigState, error) {
	var tenant types.Tenant
	err := r.db.WithContext(ctx).Select("id", "memory_config", "memory_generation").
		Where("id = ?", tenantID).First(&tenant).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	cfg := normalizedMemoryConfig(tenant.MemoryConfig)
	return &interfaces.TenantMemoryConfigState{Config: cfg, Generation: tenant.MemoryGeneration}, nil
}

func normalizedMemoryConfig(cfg *types.MemoryConfig) *types.MemoryConfig {
	if cfg == nil {
		cfg = &types.MemoryConfig{}
	} else {
		copy := *cfg
		cfg = &copy
	}
	cfg.Normalize()
	return cfg
}

func sameMemoryConfig(a, b *types.MemoryConfig) bool {
	left, _ := json.Marshal(normalizedMemoryConfig(a))
	right, _ := json.Marshal(normalizedMemoryConfig(b))
	return string(left) == string(right)
}

func (r *tenantMemoryConfigRepository) Update(
	ctx context.Context, tenantID uint64, cfg *types.MemoryConfig,
) (*interfaces.TenantMemoryConfigState, error) {
	cfg = normalizedMemoryConfig(cfg)
	var result *interfaces.TenantMemoryConfigState
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant types.Tenant
		if err := tx.Select("id", "memory_config", "memory_generation").
			Where("id = ?", tenantID).Clauses(forUpdateClause()).First(&tenant).Error; err != nil {
			return err
		}
		generation := tenant.MemoryGeneration
		if !sameMemoryConfig(tenant.MemoryConfig, cfg) {
			generation++
			if err := tx.Model(&types.Tenant{}).Where("id = ?", tenantID).
				Updates(map[string]interface{}{
					"memory_config": cfg, "memory_generation": generation, "updated_at": time.Now(),
				}).Error; err != nil {
				return err
			}
		}
		copy := *cfg
		result = &interfaces.TenantMemoryConfigState{Config: &copy, Generation: generation}
		return nil
	})
	return result, err
}
