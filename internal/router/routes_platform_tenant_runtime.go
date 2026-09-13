package router

import (
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// RegisterSystemAdminTenantRuntimeRoutes manages enterprise-owned conversation data
// through an explicit tenant scope. Shared infrastructure uses platform routes.
func RegisterSystemAdminTenantRuntimeRoutes(
	r *gin.RouterGroup,
	tenantService interfaces.TenantService,
	tenant *handler.TenantHandler,
	messages *handler.MessageHandler,
	memory *handler.TenantMemoryConfigHandler,
	capabilityPlan *handler.AICapabilityPlanHandler,
	g *rbacGuards,
) {
	tenantRuntime := r.Group(
		"/system/admin/tenants/:tenant_id",
		g.SystemAdmin(),
		middleware.BindSystemAdminTenantScope(tenantService),
	)

	tenantRuntime.GET("/chat-history-config", tenant.GetTenantChatHistoryConfig)
	tenantRuntime.PUT("/chat-history-config", func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "key", Value: "chat-history-config"})
		tenant.UpdateTenantKV(c)
	})
	tenantRuntime.GET("/chat-history-stats", messages.GetChatHistoryKBStats)

	if memory != nil {
		tenantRuntime.GET("/memory-config", memory.Get)
		tenantRuntime.PUT("/memory-config", memory.Update)
	}

	if capabilityPlan != nil {
		tenantRuntime.GET("/retrieval-processing-settings", capabilityPlan.GetPlatformRetrievalProcessingSettings)
	}

}
