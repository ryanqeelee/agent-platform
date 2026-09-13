package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
)

// RegisterCapabilityPlanAdminRoutes mounts browser-only system-admin routes.
func RegisterCapabilityPlanAdminRoutes(
	r *gin.RouterGroup,
	h *handler.CapabilityPlanAdminHandler,
	g *rbacGuards,
) {
	if h == nil {
		return
	}
	admin := r.Group("/system/admin", g.SystemAdmin())
	admin.POST("/capability-plans", h.CreatePlan)
	admin.GET("/capability-plans", h.ListPlans)
	admin.PUT("/capability-plans/default", h.SetDefaultPlan)
	admin.GET("/enterprises/:tenant_id/capability-plan", h.GetTenantPlan)
	admin.PUT("/enterprises/:tenant_id/capability-plan", h.AssignTenantPlan)
	admin.DELETE("/enterprises/:tenant_id/capability-plan", h.ClearTenantPlan)

	admin.GET("/assistant-scenario-capabilities/default", h.GetDefaultScenario)
	admin.PUT("/assistant-scenario-capabilities/default", h.SetDefaultScenario)
	admin.GET("/enterprises/:tenant_id/assistant-scenario-capabilities", h.GetTenantScenario)
	admin.PUT("/enterprises/:tenant_id/assistant-scenario-capabilities", h.SetTenantScenario)
	admin.DELETE("/enterprises/:tenant_id/assistant-scenario-capabilities", h.ClearTenantScenario)
}
