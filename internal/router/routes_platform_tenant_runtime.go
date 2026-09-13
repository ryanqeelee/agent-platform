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
// the established tenant, message, sandbox and skill handlers keep enforcing
// repository tenant filters. No API-key policy is declared, preserving the v1
// gate's default deny.
func RegisterSystemAdminTenantRuntimeRoutes(
	r *gin.RouterGroup,
	tenantService interfaces.TenantService,
	tenant *handler.TenantHandler,
	messages *handler.MessageHandler,
	sandboxConfigs *handler.SandboxConfigHandler,
	sandboxSkills *handler.SandboxSkillHandler,
	skills *handler.SkillHandler,
	customAgents *handler.CustomAgentHandler,
	system *handler.SystemHandler,
	memory *handler.TenantMemoryConfigHandler,
	capabilityPlan *handler.AICapabilityPlanHandler,
	vectorStores *handler.VectorStoreHandler,
	storageBackends *handler.StorageBackendHandler,
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

	if vectorStores != nil {
		stores := tenantRuntime.Group("/vector-stores")
		stores.GET("/types", vectorStores.ListStoreTypes)
		stores.POST("/test", vectorStores.TestStoreRaw)
		stores.POST("", vectorStores.CreateStore)
		stores.GET("", vectorStores.ListStores)
		stores.GET("/:id", vectorStores.GetStore)
		stores.PUT("/:id", vectorStores.UpdateStore)
		stores.DELETE("/:id", vectorStores.DeleteStore)
		stores.POST("/:id/test", vectorStores.TestStoreByID)
	}

	if storageBackends != nil {
		backends := tenantRuntime.Group("/storage-backends")
		backends.GET("/types", storageBackends.Types)
		backends.POST("/test", storageBackends.TestRaw)
		backends.POST("", storageBackends.Create)
		backends.GET("", storageBackends.List)
		backends.GET("/:id", storageBackends.Get)
		backends.PUT("/:id", storageBackends.Update)
		backends.DELETE("/:id", storageBackends.Delete)
		backends.POST("/:id/test", storageBackends.TestByID)
		backends.PUT("/:id/default", storageBackends.SetDefault)
	}

	skillRoutes := tenantRuntime.Group("/skills")
	skillRoutes.GET("/installer-agent", customAgents.GetSkillInstallerAgent)
	skillRoutes.PUT("/installer-agent", customAgents.UpdateSkillInstallerAgent)
	registerSkillReadHandlers(skillRoutes, skills)
	registerSkillCatalogWriteHandlers(skillRoutes.Group("/catalog"), skills)
}
