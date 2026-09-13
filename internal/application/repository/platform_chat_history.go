package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrPlatformChatHistoryConfigMissing = errors.New("platform chat-history configuration singleton is missing")
	ErrPlatformChatHistoryConfigChanged = errors.New("platform chat-history configuration changed during knowledge-base preparation")
	ErrPlatformChatHistoryModelLocked   = errors.New("message index model is locked by existing tenant indexes")
	ErrPlatformChatHistoryModelInvalid  = errors.New("selected platform embedding model is unavailable")
)

type PlatformChatHistoryRepository interface {
	GetConfig(ctx context.Context) (*types.PlatformChatHistoryConfig, error)
	UpdateConfig(
		ctx context.Context,
		config *types.PlatformChatHistoryConfig,
		actorID string,
		now time.Time,
	) (*types.PlatformChatHistoryConfig, error)
	GetTenantKnowledgeBase(ctx context.Context, tenantID uint64) (*types.KnowledgeBase, error)
	EnsureTenantKnowledgeBase(
		ctx context.Context,
		expectedModelID string,
		kb *types.KnowledgeBase,
	) (*types.KnowledgeBase, bool, error)
	Stats(ctx context.Context, tenantID *uint64) (tenantKBs, indexedMessages int64, err error)
}

type platformChatHistoryRepository struct{ db *gorm.DB }

func NewPlatformChatHistoryRepository(db *gorm.DB) PlatformChatHistoryRepository {
	return &platformChatHistoryRepository{db: db}
}

func (r *platformChatHistoryRepository) GetConfig(ctx context.Context) (*types.PlatformChatHistoryConfig, error) {
	var config types.PlatformChatHistoryConfig
	err := r.db.WithContext(ctx).
		Where("id = ?", types.PlatformChatHistoryConfigSingletonID).
		Take(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPlatformChatHistoryConfigMissing
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *platformChatHistoryRepository) UpdateConfig(
	ctx context.Context,
	config *types.PlatformChatHistoryConfig,
	actorID string,
	now time.Time,
) (*types.PlatformChatHistoryConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("platform chat-history config is required")
	}
	modelID := strings.TrimSpace(config.EmbeddingModelID)
	var updated types.PlatformChatHistoryConfig
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Assignment and model mutation share the model-row lock. Acquiring it
		// before the singleton lock makes validation atomic with model updates,
		// disablement, and deletion.
		if modelID != "" {
			var model types.Model
			if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
				Where("id = ?", modelID).
				Take(&model).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrPlatformChatHistoryModelInvalid
				}
				return err
			}
			if model.Type != types.ModelTypeEmbedding || model.Status != types.ModelStatusActive || !model.IsBuiltin {
				return ErrPlatformChatHistoryModelInvalid
			}
		}

		var current types.PlatformChatHistoryConfig
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", types.PlatformChatHistoryConfigSingletonID).
			Take(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPlatformChatHistoryConfigMissing
			}
			return err
		}

		// A bound KB may already have an asynchronous passage task in flight.
		// Once any binding exists, only that exact embedding model can be
		// selected again. Disabled+empty remains a valid way to turn indexing
		// off without discarding the private indexes.
		if modelID != "" {
			var mismatched int64
			if err := tx.Table("tenant_chat_history_indexes AS idx").
				Joins("JOIN knowledge_bases AS kb ON kb.id = idx.knowledge_base_id").
				Where("kb.embedding_model_id <> ?", modelID).
				Count(&mismatched).Error; err != nil {
				return err
			}
			if mismatched > 0 {
				return ErrPlatformChatHistoryModelLocked
			}
		}

		updates := map[string]any{
			"enabled":            config.Enabled,
			"embedding_model_id": modelID,
			"updated_by":         actorID,
			"updated_at":         now,
		}
		if err := tx.Model(&types.PlatformChatHistoryConfig{}).
			Where("id = ?", types.PlatformChatHistoryConfigSingletonID).
			Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", types.PlatformChatHistoryConfigSingletonID).
			Take(&updated).Error
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (r *platformChatHistoryRepository) GetTenantKnowledgeBase(
	ctx context.Context, tenantID uint64,
) (*types.KnowledgeBase, error) {
	var kb types.KnowledgeBase
	err := r.db.WithContext(ctx).
		Table("knowledge_bases AS kb").
		Select("kb.*").
		Joins("JOIN tenant_chat_history_indexes AS idx ON idx.knowledge_base_id = kb.id").
		Where("idx.tenant_id = ? AND kb.tenant_id = ? AND kb.deleted_at IS NULL", tenantID, tenantID).
		Take(&kb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	kb.EnsureDefaults()
	return &kb, nil
}

func (r *platformChatHistoryRepository) EnsureTenantKnowledgeBase(
	ctx context.Context,
	expectedModelID string,
	kb *types.KnowledgeBase,
) (*types.KnowledgeBase, bool, error) {
	if kb == nil || kb.TenantID == 0 || strings.TrimSpace(expectedModelID) == "" {
		return nil, false, fmt.Errorf("prepared tenant chat-history knowledge base is invalid")
	}
	var result *types.KnowledgeBase
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock order is global singleton then tenant. Config PUT takes the
		// singleton UPDATE lock, so it cannot switch policy between preparation
		// and the atomic KB+binding write.
		var config types.PlatformChatHistoryConfig
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
			Where("id = ?", types.PlatformChatHistoryConfigSingletonID).
			Take(&config).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPlatformChatHistoryConfigMissing
			}
			return err
		}
		if !config.Enabled || strings.TrimSpace(config.EmbeddingModelID) != expectedModelID {
			return ErrPlatformChatHistoryConfigChanged
		}

		var tenant struct{ ID uint64 }
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Table("tenants").Select("id").
			Where("id = ? AND deleted_at IS NULL", kb.TenantID).
			Take(&tenant).Error; err != nil {
			return err
		}

		var binding types.TenantChatHistoryIndex
		err := tx.Where("tenant_id = ?", kb.TenantID).Take(&binding).Error
		switch {
		case err == nil:
			var existing types.KnowledgeBase
			if err := tx.Where(
				"id = ? AND tenant_id = ? AND embedding_model_id = ?",
				binding.KnowledgeBaseID, kb.TenantID, expectedModelID,
			).
				Take(&existing).Error; err != nil {
				return err
			}
			result = &existing
			return nil
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}

		if kb.EmbeddingModelID != expectedModelID {
			return ErrPlatformChatHistoryConfigChanged
		}
		if err := tx.Create(kb).Error; err != nil {
			return err
		}
		binding = types.TenantChatHistoryIndex{
			TenantID:        kb.TenantID,
			KnowledgeBaseID: kb.ID,
			CreatedAt:       kb.CreatedAt,
			UpdatedAt:       kb.UpdatedAt,
		}
		if err := tx.Create(&binding).Error; err != nil {
			return err
		}
		created = true
		copy := *kb
		result = &copy
		return nil
	})
	return result, created, err
}

func (r *platformChatHistoryRepository) Stats(
	ctx context.Context, tenantID *uint64,
) (tenantKBs, indexedMessages int64, err error) {
	bindings := r.db.WithContext(ctx).Table("tenant_chat_history_indexes AS idx")
	if tenantID != nil {
		bindings = bindings.Where("idx.tenant_id = ?", *tenantID)
	}
	if err = bindings.Count(&tenantKBs).Error; err != nil {
		return 0, 0, err
	}
	knowledge := r.db.WithContext(ctx).
		Table("tenant_chat_history_indexes AS idx").
		Joins("JOIN knowledges AS k ON k.knowledge_base_id = idx.knowledge_base_id AND k.deleted_at IS NULL")
	if tenantID != nil {
		knowledge = knowledge.Where("idx.tenant_id = ?", *tenantID)
	}
	err = knowledge.Count(&indexedMessages).Error
	return tenantKBs, indexedMessages, err
}
