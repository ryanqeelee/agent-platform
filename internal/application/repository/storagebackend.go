package repository

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type storageBackendRepository struct{ db *gorm.DB }

func NewStorageBackendRepository(db *gorm.DB) interfaces.StorageBackendRepository {
	return &storageBackendRepository{db: db}
}

func (r *storageBackendRepository) Create(ctx context.Context, backend *types.StorageBackend) error {
	return r.db.WithContext(ctx).Create(backend).Error
}

func (r *storageBackendRepository) GetByID(ctx context.Context, id string) (*types.StorageBackend, error) {
	var backend types.StorageBackend
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&backend).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &backend, nil
}

func (r *storageBackendRepository) GetDefault(ctx context.Context) (*types.StorageBackend, error) {
	var backend types.StorageBackend
	if err := r.db.WithContext(ctx).Where("is_default = ?", true).First(&backend).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &backend, nil
}

func (r *storageBackendRepository) List(ctx context.Context) ([]*types.StorageBackend, error) {
	var backends []*types.StorageBackend
	err := r.db.WithContext(ctx).Order("created_at DESC").Find(&backends).Error
	return backends, err
}

func (r *storageBackendRepository) Update(ctx context.Context, backend *types.StorageBackend) error {
	return r.db.WithContext(ctx).Model(&types.StorageBackend{}).
		Where("id = ?", backend.ID).
		Select("name", "config", "status", "updated_at").Updates(backend).Error
}

func (r *storageBackendRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&types.StorageBackend{}).Error
}
