package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// webSearchProviderRepository implements the WebSearchProviderRepository interface
type webSearchProviderRepository struct {
	db *gorm.DB
}

// NewWebSearchProviderRepository creates a new web search provider repository
func NewWebSearchProviderRepository(db *gorm.DB) interfaces.WebSearchProviderRepository {
	return &webSearchProviderRepository{db: db}
}

// Create creates a new web search provider
func (r *webSearchProviderRepository) Create(ctx context.Context, provider *types.WebSearchProviderEntity) error {
	return r.db.WithContext(ctx).Create(provider).Error
}

func (r *webSearchProviderRepository) GetDefault(ctx context.Context) (*types.WebSearchProviderEntity, error) {
	var provider types.WebSearchProviderEntity
	if err := r.db.WithContext(ctx).Where("is_default = ?", true).Take(&provider).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &provider, nil
}

func (r *webSearchProviderRepository) SetDefault(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockWebSearchDefault(tx); err != nil {
			return err
		}
		var provider types.WebSearchProviderEntity
		query := tx.Where("id = ?", id)
		if tx.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.Take(&provider).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("web search provider not found")
			}
			return err
		}
		if err := tx.Model(&types.WebSearchProviderEntity{}).
			Where("is_default = ? AND id <> ?", true, id).
			Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Model(&types.WebSearchProviderEntity{}).
			Where("id = ?", id).
			Update("is_default", true).Error
	})
}

func lockWebSearchDefault(tx *gorm.DB) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(0x5745425345415243)).Error
}

// GetByID retrieves a platform web search provider by ID.
func (r *webSearchProviderRepository) GetByID(ctx context.Context, id string) (*types.WebSearchProviderEntity, error) {
	var provider types.WebSearchProviderEntity
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&provider).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &provider, nil
}

// List lists all platform web search providers.
func (r *webSearchProviderRepository) List(ctx context.Context) ([]*types.WebSearchProviderEntity, error) {
	var providers []*types.WebSearchProviderEntity
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

// Update updates a web search provider
func (r *webSearchProviderRepository) Update(ctx context.Context, provider *types.WebSearchProviderEntity) error {
	return r.db.WithContext(ctx).Model(&types.WebSearchProviderEntity{}).Where(
		"id = ?", provider.ID,
	).Select("name", "provider", "description", "parameters", "updated_at").Updates(provider).Error
}

// Delete soft-deletes a web search provider
func (r *webSearchProviderRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockWebSearchDefault(tx); err != nil {
			return err
		}
		var provider types.WebSearchProviderEntity
		query := tx.Where("id = ?", id)
		if tx.Dialector.Name() == "postgres" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.Take(&provider).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("web search provider not found")
			}
			return err
		}
		if provider.IsDefault {
			return fmt.Errorf("default web search provider cannot be deleted")
		}
		return tx.Delete(&provider).Error
	})
}
