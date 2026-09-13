package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

// modelRepository implements the model repository interface
type modelRepository struct {
	db *gorm.DB
}

// NewModelRepository creates a new model repository
func NewModelRepository(db *gorm.DB) interfaces.ModelRepository {
	return &modelRepository{db: db}
}

// Create creates a new model
func (r *modelRepository) Create(ctx context.Context, m *types.Model) error {
	return r.db.WithContext(ctx).Create(m).Error
}

// GetByID retrieves a model by ID
func (r *modelRepository) GetByID(ctx context.Context, tenantID uint64, id string) (*types.Model, error) {
	var m types.Model
	if err := r.db.WithContext(ctx).Where("id = ?", id).Where(
		"(tenant_id = ? OR is_builtin = true)", tenantID,
	).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// List lists models with optional filtering
func (r *modelRepository) List(
	ctx context.Context, tenantID uint64, modelType types.ModelType, source types.ModelSource,
) ([]*types.Model, error) {
	var models []*types.Model
	query := r.db.WithContext(ctx).Where(
		"(tenant_id = ? OR is_builtin = true)", tenantID,
	)

	if modelType != "" {
		query = query.Where("type = ?", modelType)
	}

	if source != "" {
		query = query.Where("source = ?", source)
	}

	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	return models, nil
}

// Update updates a model
func (r *modelRepository) Update(ctx context.Context, m *types.Model) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var stored types.Model
		if err := tx.WithContext(ctx).Where("id = ? AND tenant_id = ?", m.ID, m.TenantID).
			Clauses(forUpdateClause()).Take(&stored).Error; err != nil {
			return err
		}
		var runtime types.PlatformMemoryRuntimeConfig
		if err := tx.WithContext(ctx).Where("id = ?", types.PlatformMemoryRuntimeConfigSingletonID).
			Take(&runtime).Error; err != nil {
			return err
		}
		if runtime.Runtime != nil && runtime.Runtime.EmbeddingModelID == m.ID &&
			(!m.IsBuiltin || m.Status != types.ModelStatusActive ||
				types.EmbeddingModelSemanticIdentityChanged(&stored, m)) {
			return fmt.Errorf("%w: %s is the platform memory embedding model", types.ErrModelPlatformRuntimeBinding, m.ID)
		}
		if runtime.Runtime != nil && runtime.Runtime.ExtractModelID == m.ID &&
			(!m.IsBuiltin || m.Status != types.ModelStatusActive || m.Type != types.ModelTypeKnowledgeQA) {
			return fmt.Errorf("%w: %s is the platform memory extraction model", types.ErrModelPlatformRuntimeBinding, m.ID)
		}
		chatBound, err := platformChatEmbeddingModelBound(ctx, tx, m.ID)
		if err != nil {
			return err
		}
		if chatBound && (!m.IsBuiltin || m.Status != types.ModelStatusActive ||
			types.EmbeddingModelSemanticIdentityChanged(&stored, m)) {
			return fmt.Errorf("%w: %s is bound by platform chat history", types.ErrModelPlatformRuntimeBinding, m.ID)
		}
		// Use Select to explicitly update all fields, including zero values like false.
		return tx.WithContext(ctx).Model(&types.Model{}).Where(
			"id = ? AND tenant_id = ?", m.ID, m.TenantID,
		).Select("*").Updates(m).Error
	})
}

// Delete deletes a model
func (r *modelRepository) Delete(ctx context.Context, tenantID uint64, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var stored types.Model
		if err := tx.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).
			Clauses(forUpdateClause()).Take(&stored).Error; err != nil {
			return err
		}
		var runtime types.PlatformMemoryRuntimeConfig
		if err := tx.WithContext(ctx).Where("id = ?", types.PlatformMemoryRuntimeConfigSingletonID).
			Take(&runtime).Error; err != nil {
			return err
		}
		if runtime.Runtime != nil &&
			(runtime.Runtime.EmbeddingModelID == id || runtime.Runtime.ExtractModelID == id) {
			return fmt.Errorf("%w: %s is bound by platform memory runtime", types.ErrModelPlatformRuntimeBinding, id)
		}
		chatBound, err := platformChatEmbeddingModelBound(ctx, tx, id)
		if err != nil {
			return err
		}
		if chatBound {
			return fmt.Errorf("%w: %s is bound by platform chat history", types.ErrModelPlatformRuntimeBinding, id)
		}
		return tx.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).
			Delete(&types.Model{}).Error
	})
}

// platformChatEmbeddingModelBound is called only after the target model row
// is locked. Assignment writers lock that model row first, so these fresh
// statements observe any preceding assignment and prevent a concurrent new
// assignment from completing until the mutation has committed.
func platformChatEmbeddingModelBound(ctx context.Context, tx *gorm.DB, modelID string) (bool, error) {
	var count int64
	if err := tx.WithContext(ctx).Table("platform_chat_history_config").
		Where("embedding_model_id = ?", modelID).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if err := tx.WithContext(ctx).Table("tenant_chat_history_indexes AS idx").
		Joins("JOIN knowledge_bases AS kb ON kb.id = idx.knowledge_base_id").
		Where("kb.embedding_model_id = ?", modelID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *modelRepository) PlatformMemoryModelBindings(
	ctx context.Context, modelID string,
) ([]types.ModelUsageBinding, error) {
	var runtime types.PlatformMemoryRuntimeConfig
	if err := r.db.WithContext(ctx).Where("id = ?", types.PlatformMemoryRuntimeConfigSingletonID).
		Take(&runtime).Error; err != nil {
		return nil, err
	}
	bindings := make([]types.ModelUsageBinding, 0, 2)
	if runtime.Runtime != nil && runtime.Runtime.EmbeddingModelID == modelID {
		bindings = append(bindings, types.ModelUsageBindingEmbeddingModel)
	}
	if runtime.Runtime != nil && runtime.Runtime.ExtractModelID == modelID {
		bindings = append(bindings, types.ModelUsageBindingExtractModel)
	}
	return bindings, nil
}

// ClearDefaultByType clears the default flag for all models of a specific type
// This is a batch operation that updates all matching records in one query
func (r *modelRepository) ClearDefaultByType(
	ctx context.Context,
	tenantID uint,
	modelType types.ModelType,
	excludeID string,
) error {
	query := r.db.WithContext(ctx).Model(&types.Model{}).Where(
		"tenant_id = ? AND type = ? AND is_default = ?", tenantID, modelType, true,
	)

	// If excludeID is provided, exclude that model from the update
	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}

	// Batch update: set is_default to false for all matching records
	return query.Update("is_default", false).Error
}
