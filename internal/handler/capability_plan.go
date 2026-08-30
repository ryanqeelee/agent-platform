package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type AICapabilityPlanHandler struct {
	resolver                    interfaces.AICapabilityPlanResolver
	modelRuntimeResolver        interfaces.PlatformModelRuntimeSettingsResolver
	retrievalProcessingResolver interfaces.PlatformRetrievalProcessingSettingsResolver
}

func NewAICapabilityPlanHandler(
	resolver interfaces.AICapabilityPlanResolver,
	modelRuntimeResolver interfaces.PlatformModelRuntimeSettingsResolver,
	retrievalProcessingResolver interfaces.PlatformRetrievalProcessingSettingsResolver,
) *AICapabilityPlanHandler {
	return &AICapabilityPlanHandler{
		resolver:                    resolver,
		modelRuntimeResolver:        modelRuntimeResolver,
		retrievalProcessingResolver: retrievalProcessingResolver,
	}
}

func (h *AICapabilityPlanHandler) GetPlatformModelRuntimeSettings(c *gin.Context) {
	settings, err := h.modelRuntimeResolver.ResolvePlatformModelRuntimeSettings(
		c.Request.Context(),
		c.GetUint64(types.TenantIDContextKey.String()),
	)
	if err != nil || settings == nil {
		c.Error(errors.NewServiceUnavailableError("平台运行配置暂不可用"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": settings})
}

func (h *AICapabilityPlanHandler) GetPlatformRetrievalProcessingSettings(c *gin.Context) {
	settings, err := h.retrievalProcessingResolver.ResolvePlatformRetrievalProcessingSettings(
		c.Request.Context(),
		c.GetUint64(types.TenantIDContextKey.String()),
	)
	if err != nil || settings == nil {
		c.Error(errors.NewServiceUnavailableError("平台检索与处理配置暂不可用"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": settings})
}

func (h *AICapabilityPlanHandler) GetEnterpriseProjection(c *gin.Context) {
	resolution, err := h.resolver.Resolve(c.Request.Context(), c.GetUint64(types.TenantIDContextKey.String()))
	if err != nil || resolution == nil {
		c.Error(errors.NewServiceUnavailableError("AI 能力暂不可用"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resolution.Enterprise})
}
