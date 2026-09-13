package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PlatformMemoryRuntimeConfigService is the single deployment authority for
// memory tuning and model selection.
type PlatformMemoryRuntimeConfigService struct {
	repo repository.PlatformMemoryRuntimeConfigRepository
	now  func() time.Time
}

func NewPlatformMemoryRuntimeConfigService(
	repo repository.PlatformMemoryRuntimeConfigRepository,
) *PlatformMemoryRuntimeConfigService {
	return &PlatformMemoryRuntimeConfigService{repo: repo, now: time.Now}
}

func (s *PlatformMemoryRuntimeConfigService) Get(
	ctx context.Context,
) (*types.MemoryRuntimeConfig, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("platform memory runtime repository is not configured")
	}
	row, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	if row == nil || row.Runtime == nil {
		return nil, fmt.Errorf("platform memory runtime singleton is missing")
	}
	out := *row.Runtime
	out.Normalize()
	return &out, nil
}

func validateMemoryRuntimeConfig(cfg *types.MemoryRuntimeConfig) error {
	if cfg == nil {
		return fmt.Errorf("memory runtime config is required")
	}
	if len(cfg.ExtractModelID) > types.ModelIDMaxLen {
		return fmt.Errorf("extract_model_id is too long")
	}
	if len(cfg.EmbeddingModelID) > types.ModelIDMaxLen {
		return fmt.Errorf("embedding_model_id is too long")
	}
	if cfg.MaxItems < 0 || cfg.MaxItems > 2000 {
		return fmt.Errorf("max_items must be between 0 and 2000")
	}
	if cfg.ExtractDelaySeconds < 0 || cfg.ExtractDelaySeconds > types.MaxMemoryExtractDelaySeconds {
		return fmt.Errorf("extract_delay_seconds must be between 0 and %d", types.MaxMemoryExtractDelaySeconds)
	}
	if cfg.ExtractMinIntervalSeconds < 0 || cfg.ExtractMinIntervalSeconds > types.MaxMemoryExtractMinIntervalSeconds {
		return fmt.Errorf("extract_min_interval_seconds must be between 0 and %d", types.MaxMemoryExtractMinIntervalSeconds)
	}
	if cfg.InterestThreshold < 0 || cfg.InterestThreshold > types.MaxMemoryInterestThreshold {
		return fmt.Errorf("interest_threshold must be between 0 and %d", types.MaxMemoryInterestThreshold)
	}
	if len([]rune(cfg.ExtractInstructions)) > types.MaxMemoryExtractInstructionsRunes {
		return fmt.Errorf("extract_instructions must be at most %d characters", types.MaxMemoryExtractInstructionsRunes)
	}
	return nil
}

func (s *PlatformMemoryRuntimeConfigService) validateModel(
	ctx context.Context, tx *gorm.DB, id string, wantType types.ModelType, field string,
) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	var model types.Model
	err := tx.WithContext(ctx).Where("id = ? AND (tenant_id = 0 OR is_builtin = true)", id).
		Clauses(clause.Locking{Strength: "SHARE"}).Take(&model).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || !model.IsBuiltin || model.Status != types.ModelStatusActive || model.Type != wantType {
		return apperrors.NewValidationError(fmt.Sprintf(
			"%s must reference an active platform-owned %s model", field, wantType,
		))
	}
	return nil
}

func (s *PlatformMemoryRuntimeConfigService) Update(
	ctx context.Context, incoming *types.MemoryRuntimeConfig,
) (*types.MemoryRuntimeConfig, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("platform memory runtime repository is not configured")
	}
	if err := validateMemoryRuntimeConfig(incoming); err != nil {
		return nil, apperrors.NewValidationError(err.Error())
	}
	actorID, ok := types.UserIDFromContext(ctx)
	if !types.IsSystemAdminFromContext(ctx) || !ok || strings.TrimSpace(actorID) == "" {
		return nil, apperrors.NewForbiddenError("system administrator identity is required")
	}
	runtime := *incoming
	runtime.Normalize()
	row, err := s.repo.Update(ctx, &runtime, actorID, s.now(), func(tx *gorm.DB) error {
		// Stable order avoids deadlocks when both pins change together. Model
		// mutations lock the same rows before inspecting the singleton.
		checks := []struct {
			id, field string
			type_     types.ModelType
		}{
			{runtime.EmbeddingModelID, "embedding_model_id", types.ModelTypeEmbedding},
			{runtime.ExtractModelID, "extract_model_id", types.ModelTypeKnowledgeQA},
		}
		sort.Slice(checks, func(i, j int) bool { return checks[i].id < checks[j].id })
		for _, check := range checks {
			if err := s.validateModel(ctx, tx, check.id, check.type_, check.field); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if row == nil || row.Runtime == nil {
		return nil, fmt.Errorf("platform memory runtime singleton is missing")
	}
	out := *row.Runtime
	return &out, nil
}
