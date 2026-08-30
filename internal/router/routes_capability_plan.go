package router

import (
	"net/http"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func RegisterCapabilityPlanRoutes(
	r *gin.RouterGroup,
	h *handler.AICapabilityPlanHandler,
	g *rbacGuards,
) {
	r.GET(
		"/tenants/:id/ai-capability-plan",
		g.PathTenantMatch(),
		g.Viewer(),
		h.GetEnterpriseProjection,
	)
	g.apiKeyRoute(
		r,
		http.MethodGet,
		"/platform/model-runtime-settings",
		apiKeyPlatform(types.APIKeyCapabilityManageModels),
		g.SystemAdmin(),
		h.GetPlatformModelRuntimeSettings,
	)
	g.apiKeyRoute(
		r,
		http.MethodGet,
		"/platform/retrieval-processing-settings",
		apiKeyPlatform(types.APIKeyCapabilitySystemSettingsRead),
		g.SystemAdmin(),
		h.GetPlatformRetrievalProcessingSettings,
	)
}
