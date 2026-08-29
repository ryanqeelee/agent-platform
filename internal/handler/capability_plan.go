package handler

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type AICapabilityPlanHandler struct {
	resolver interfaces.AICapabilityPlanResolver
}

func NewAICapabilityPlanHandler(resolver interfaces.AICapabilityPlanResolver) *AICapabilityPlanHandler {
	return &AICapabilityPlanHandler{resolver: resolver}
}

func (h *AICapabilityPlanHandler) GetEnterpriseProjection(c *gin.Context) {
	resolution, err := h.resolver.Resolve(c.Request.Context(), c.GetUint64(types.TenantIDContextKey.String()))
	if err != nil || resolution == nil {
		c.Error(errors.NewServiceUnavailableError("AI 能力暂不可用"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": resolution.Enterprise})
}
