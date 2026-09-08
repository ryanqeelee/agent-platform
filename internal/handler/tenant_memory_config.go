package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type TenantMemoryConfigHandler struct {
	service interfaces.TenantMemoryConfigService
}

func NewTenantMemoryConfigHandler(service interfaces.TenantMemoryConfigService) *TenantMemoryConfigHandler {
	return &TenantMemoryConfigHandler{service: service}
}

func (h *TenantMemoryConfigHandler) Get(c *gin.Context) {
	state, err := h.service.Get(c.Request.Context())
	if err != nil || state == nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": state.Config, "generation": state.Generation})
}

func (h *TenantMemoryConfigHandler) Update(c *gin.Context) {
	var cfg types.MemoryConfig
	if err := decodeStrictMemoryJSON(c, &cfg); err != nil {
		c.Error(errors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}
	if err := validateTenantMemoryConfig(&cfg); err != nil {
		c.Error(errors.NewBadRequestError(err.Error()))
		return
	}
	state, err := h.service.Update(c.Request.Context(), &cfg)
	if err != nil || state == nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true, "data": state.Config, "generation": state.Generation,
		"message": "Memory configuration updated successfully",
	})
}

func validateTenantMemoryConfig(cfg *types.MemoryConfig) error {
	if cfg.WriteMode != "" && cfg.WriteMode != types.MemoryWriteExplicitOnly && cfg.WriteMode != types.MemoryWriteAuto {
		return fmt.Errorf("write_mode must be explicit_only or auto")
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
	if len(cfg.EmbeddingModelID) > 64 {
		return fmt.Errorf("embedding_model_id is too long")
	}
	if cfg.InterestThreshold < 0 || cfg.InterestThreshold > types.MaxMemoryInterestThreshold {
		return fmt.Errorf("interest_threshold must be between 1 and %d", types.MaxMemoryInterestThreshold)
	}
	if len([]rune(cfg.ExtractInstructions)) > types.MaxMemoryExtractInstructionsRunes {
		return fmt.Errorf("extract_instructions must be at most %d characters", types.MaxMemoryExtractInstructionsRunes)
	}
	cfg.Normalize()
	return nil
}

func (h *TenantMemoryConfigHandler) fail(c *gin.Context, err error) {
	if err != nil {
		logger.ErrorWithFields(c.Request.Context(), err, nil)
	}
	c.Error(errors.NewInternalServerError("Failed to access memory config"))
}
