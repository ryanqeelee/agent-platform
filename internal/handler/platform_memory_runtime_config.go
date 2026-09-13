package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/application/service"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

type PlatformMemoryRuntimeConfigHandler struct {
	service *service.PlatformMemoryRuntimeConfigService
}

func NewPlatformMemoryRuntimeConfigHandler(
	service *service.PlatformMemoryRuntimeConfigService,
) *PlatformMemoryRuntimeConfigHandler {
	return &PlatformMemoryRuntimeConfigHandler{service: service}
}

// Get returns the deployment-wide memory runtime configuration.
// @Summary      获取平台记忆运行配置
// @Tags         System Admin
// @Produce      json
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /system/admin/memory-runtime-config [get]
func (h *PlatformMemoryRuntimeConfigHandler) Get(c *gin.Context) {
	runtime, err := h.service.Get(c.Request.Context())
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": runtime})
}

// Update replaces the deployment-wide memory runtime configuration.
// @Summary      更新平台记忆运行配置
// @Tags         System Admin
// @Accept       json
// @Produce      json
// @Param        body body types.MemoryRuntimeConfig true "记忆运行配置"
// @Success      200 {object} map[string]interface{}
// @Security     Bearer
// @Router       /system/admin/memory-runtime-config [put]
func (h *PlatformMemoryRuntimeConfigHandler) Update(c *gin.Context) {
	var runtime types.MemoryRuntimeConfig
	if err := decodeStrictMemoryJSON(c, &runtime); err != nil {
		c.Error(apperrors.NewValidationError("Invalid request data").WithDetails(err.Error()))
		return
	}
	updated, err := h.service.Update(c.Request.Context(), &runtime)
	if err != nil {
		if appErr, ok := apperrors.IsAppError(err); ok {
			c.Error(appErr)
			return
		}
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true, "data": updated,
		"message": "Memory runtime configuration updated successfully",
	})
}

func (h *PlatformMemoryRuntimeConfigHandler) fail(c *gin.Context, err error) {
	logger.ErrorWithFields(c.Request.Context(), err, nil)
	c.Error(apperrors.NewInternalServerError("Failed to access memory runtime config"))
}
