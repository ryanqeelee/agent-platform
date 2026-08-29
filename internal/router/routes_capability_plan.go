package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
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
}
