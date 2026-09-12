package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// RegisterSystemAdminTenantRuntimeRoutes exposes tenant-filtered platform
// runtime configuration through an explicit path scope. The platform identity
// remains tenantless; BindSystemAdminTenantScope only annotates this request so
// the established sandbox and skill handlers keep enforcing repository tenant
// filters. No API-key policy is declared, preserving the v1 gate's default deny.
func RegisterSystemAdminTenantRuntimeRoutes(
	r *gin.RouterGroup,
	tenantService interfaces.TenantService,
	sandboxConfigs *handler.SandboxConfigHandler,
	sandboxSkills *handler.SandboxSkillHandler,
	skills *handler.SkillHandler,
	system *handler.SystemHandler,
	g *rbacGuards,
) {
	tenantRuntime := r.Group(
		"/system/admin/tenants/:tenant_id",
		g.SystemAdmin(),
		middleware.BindSystemAdminTenantScope(tenantService),
	)

	configs := tenantRuntime.Group("/sandbox-configs")
	registerSandboxConfigHandlers(configs, sandboxConfigs, sandboxSkills)
	configs.POST("/check", system.CheckSandboxConfig)

	skillRoutes := tenantRuntime.Group("/skills")
	registerSkillReadHandlers(skillRoutes, skills)
	registerSkillCatalogWriteHandlers(skillRoutes.Group("/catalog"), skills)
}
