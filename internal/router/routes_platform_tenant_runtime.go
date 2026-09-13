package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// RegisterSystemAdminTenantRuntimeRoutes exposes the assigned retrieval plan
// for an explicitly selected enterprise. Shared policy uses platform routes.
func RegisterSystemAdminTenantRuntimeRoutes(
	r *gin.RouterGroup,
	tenantService interfaces.TenantService,
	capabilityPlan *handler.AICapabilityPlanHandler,
	g *rbacGuards,
) {
	tenantRuntime := r.Group(
		"/system/admin/tenants/:tenant_id",
		g.SystemAdmin(),
		middleware.BindSystemAdminTenantScope(tenantService),
	)

	if capabilityPlan != nil {
		tenantRuntime.GET("/retrieval-processing-settings", capabilityPlan.GetPlatformRetrievalProcessingSettings)
	}

}
