package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// PlatformChatHistoryService owns the deployment-wide message-index policy.
// Tenant message services consume the same singleton through RuntimeConfig;
// no enterprise policy fallback exists.
type PlatformChatHistoryService struct {
	repo   repository.PlatformChatHistoryRepository
	models interfaces.ModelService
	now    func() time.Time
}

func NewPlatformChatHistoryService(
	repo repository.PlatformChatHistoryRepository,
	models interfaces.ModelService,
) *PlatformChatHistoryService {
	return &PlatformChatHistoryService{repo: repo, models: models, now: time.Now}
}

func (s *PlatformChatHistoryService) RuntimeConfig(
	ctx context.Context,
) (*types.PlatformChatHistoryConfig, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("platform chat-history repository is not configured")
	}
	return s.repo.GetConfig(ctx)
}

func (s *PlatformChatHistoryService) Update(
	ctx context.Context, incoming *types.PlatformChatHistoryConfig,
) (*types.PlatformChatHistoryConfig, error) {
	if s == nil || s.repo == nil || s.models == nil {
		return nil, fmt.Errorf("platform chat-history service is not configured")
	}
	if incoming == nil {
		return nil, apperrors.NewValidationError("message index configuration is required")
	}
	actorID, ok := types.UserIDFromContext(ctx)
	if !ok || strings.TrimSpace(actorID) == "" || !types.IsSystemAdminFromContext(ctx) {
		return nil, apperrors.NewForbiddenError("system administrator identity is required")
	}
	incoming.EmbeddingModelID = strings.TrimSpace(incoming.EmbeddingModelID)
	if incoming.Enabled && incoming.EmbeddingModelID == "" {
		return nil, apperrors.NewValidationError("select an active platform embedding model before enabling message indexing")
	}
	if incoming.EmbeddingModelID != "" {
		model, err := s.models.GetModelByID(ctx, incoming.EmbeddingModelID)
		if err != nil || model == nil {
			return nil, apperrors.NewValidationError("selected platform embedding model is unavailable")
		}
		if model.Type != types.ModelTypeEmbedding {
			return nil, apperrors.NewValidationError("selected model must be an embedding model")
		}
		if model.Status != types.ModelStatusActive {
			return nil, apperrors.NewValidationError("selected platform embedding model must be active")
		}
		if !model.IsBuiltin {
			return nil, apperrors.NewValidationError("selected embedding model is not platform-scoped")
		}
	}
	updated, err := s.repo.UpdateConfig(ctx, incoming, actorID, s.now())
	if errors.Is(err, repository.ErrPlatformChatHistoryModelLocked) {
		return nil, apperrors.NewValidationError(
			"message index model is locked by existing tenant indexes; clear those internal indexes before selecting another embedding model",
		)
	}
	if errors.Is(err, repository.ErrPlatformChatHistoryModelInvalid) {
		return nil, apperrors.NewValidationError("selected platform embedding model is unavailable")
	}
	return updated, err
}

func (s *PlatformChatHistoryService) Stats(
	ctx context.Context,
) (*types.PlatformChatHistoryStats, error) {
	config, err := s.RuntimeConfig(ctx)
	if err != nil {
		return nil, err
	}
	tenantKBs, indexed, err := s.repo.Stats(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &types.PlatformChatHistoryStats{
		Enabled:                  config.Enabled,
		EmbeddingModelID:         config.EmbeddingModelID,
		TenantKnowledgeBaseCount: tenantKBs,
		IndexedMessageCount:      indexed,
		HasIndexedMessages:       indexed > 0,
	}, nil
}

func (s *PlatformChatHistoryService) TenantKnowledgeBase(
	ctx context.Context, tenantID uint64,
) (*types.KnowledgeBase, error) {
	return s.repo.GetTenantKnowledgeBase(ctx, tenantID)
}

func (s *PlatformChatHistoryService) TenantStats(
	ctx context.Context, tenantID uint64,
) (*types.ChatHistoryKBStats, error) {
	config, err := s.RuntimeConfig(ctx)
	if err != nil {
		return nil, err
	}
	stats := &types.ChatHistoryKBStats{
		Enabled:          config.Enabled,
		EmbeddingModelID: config.EmbeddingModelID,
	}
	kb, err := s.repo.GetTenantKnowledgeBase(ctx, tenantID)
	if err != nil || kb == nil {
		return stats, err
	}
	_, indexed, err := s.repo.Stats(ctx, &tenantID)
	if err != nil {
		return nil, err
	}
	stats.KnowledgeBaseID = kb.ID
	stats.KnowledgeBaseName = kb.Name
	stats.IndexedMessageCount = indexed
	stats.HasIndexedMessages = indexed > 0
	return stats, nil
}
