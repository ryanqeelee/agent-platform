package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/utils"
)

// PlatformParserConfigService is the single authority for deployment-wide
// parser connections, credentials, and default parser-selection rules.
type PlatformParserConfigService struct {
	repo repository.PlatformParserConfigRepository
	now  func() time.Time
}

func NewPlatformParserConfigService(repo repository.PlatformParserConfigRepository) *PlatformParserConfigService {
	return &PlatformParserConfigService{repo: repo, now: time.Now}
}

// GetRuntime returns the unmasked configuration used by parser consumers.
// A missing singleton means no platform override: providers use their built-in
// defaults and the shared DocReader continues to use DOCREADER_* environment
// configuration. This explicit empty value avoids inventing a tenant fallback.
func (s *PlatformParserConfigService) GetRuntime(ctx context.Context) (*types.ParserEngineConfig, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("platform parser configuration repository is not configured")
	}
	row, err := s.repo.Get(ctx)
	if err != nil {
		return nil, err
	}
	if row == nil || row.Config == nil {
		return &types.ParserEngineConfig{}, nil
	}
	out := *row.Config
	return &out, nil
}

// GetRedacted returns the management projection with configured secrets masked.
func (s *PlatformParserConfigService) GetRedacted(ctx context.Context) (*types.ParserEngineConfig, error) {
	cfg, err := s.GetRuntime(ctx)
	if err != nil {
		return nil, err
	}
	return types.ParserEngineConfigForResponse(cfg, true), nil
}

// Update preserves masked credentials, validates outbound endpoints, and
// persists one global row. Secret-bearing writes require SYSTEM_AES_KEY so the
// ParserEngineConfig Value hook cannot downgrade them to plaintext.
func (s *PlatformParserConfigService) Update(
	ctx context.Context,
	incoming *types.ParserEngineConfig,
) (*types.ParserEngineConfig, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("platform parser configuration repository is not configured")
	}
	if incoming == nil {
		return nil, apperrors.NewValidationError("parser engine config is required")
	}
	actorID, ok := types.UserIDFromContext(ctx)
	if !ok || strings.TrimSpace(actorID) == "" {
		return nil, apperrors.NewForbiddenError("system administrator identity is required")
	}
	existing, err := s.GetRuntime(ctx)
	if err != nil {
		return nil, err
	}
	merged := types.MergeParserEngineConfigForUpdate(incoming, existing)
	if err := validateParserEngineOverrideURLs(merged.ToOverridesMap()); err != nil {
		return nil, apperrors.NewValidationError(err.Error())
	}
	if parserConfigHasSecrets(merged) && utils.GetAESKey() == nil {
		return nil, apperrors.NewValidationError(
			"SYSTEM_AES_KEY is not configured; refusing to store parser credentials in plaintext",
		)
	}
	row, err := s.repo.Upsert(ctx, merged, actorID, s.now())
	if err != nil {
		return nil, err
	}
	return types.ParserEngineConfigForResponse(row.Config, true), nil
}

func parserConfigHasSecrets(config *types.ParserEngineConfig) bool {
	return config != nil && (strings.TrimSpace(config.MinerUAPIKey) != "" ||
		strings.TrimSpace(config.PaddleOCRVLCloudToken) != "")
}
