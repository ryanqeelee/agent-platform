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
	mcp *handler.MCPServiceHandler,
	mcpCredentials *handler.MCPCredentialsHandler,
	webSearch *handler.WebSearchProviderHandler,
	webSearchCredentials *handler.WebSearchProviderCredentialsHandler,
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

	tenantRuntime.GET("/parser-engine-config", func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "key", Value: "parser-engine-config"})
		tenant.GetTenantKV(c)
	})
	tenantRuntime.PUT("/parser-engine-config", func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "key", Value: "parser-engine-config"})
		tenant.UpdateTenantKV(c)
	})
	if capabilityPlan != nil {
		tenantRuntime.GET("/retrieval-processing-settings", capabilityPlan.GetPlatformRetrievalProcessingSettings)
	}

	if webSearch != nil && webSearchCredentials != nil {
		providers := tenantRuntime.Group("/web-search-providers")
		providers.GET("/types", webSearch.ListProviderTypes)
		providers.POST("/test", webSearch.TestProviderRaw)
		providers.POST("", webSearch.CreateProvider)
		providers.GET("", webSearch.ListProviders)
		providers.GET("/:id", webSearch.GetProvider)
		providers.PUT("/:id", webSearch.UpdateProvider)
		providers.DELETE("/:id", webSearch.DeleteProvider)
		providers.PUT("/:id/credentials", webSearchCredentials.Put)
		providers.DELETE("/:id/credentials/:field", webSearchCredentials.DeleteField)
		providers.POST("/:id/test", webSearch.TestProviderByID)
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

	if mcp != nil && mcpCredentials != nil {
		services := tenantRuntime.Group("/mcp-services")
		services.POST("", mcp.CreateMCPService)
		services.GET("", mcp.ListMCPServices)
		services.GET("/:id", mcp.GetMCPService)
		services.PUT("/:id", mcp.UpdateMCPService)
		services.DELETE("/:id", mcp.DeleteMCPService)
		services.POST("/:id/test", mcp.TestMCPService)
		services.GET("/:id/tools", mcp.GetMCPServiceTools)
		services.GET("/:id/resources", mcp.GetMCPServiceResources)
		services.PUT("/:id/credentials", mcpCredentials.Put)
		services.DELETE("/:id/credentials/:field", mcpCredentials.DeleteField)
		services.GET("/:id/tool-approvals", mcp.ListMCPToolApprovals)
		services.PUT("/:id/tool-approvals/:tool_name", mcp.SetMCPToolApproval)
	}

	skillRoutes := tenantRuntime.Group("/skills")
	skillRoutes.GET("/installer-agent", customAgents.GetSkillInstallerAgent)
	skillRoutes.PUT("/installer-agent", customAgents.UpdateSkillInstallerAgent)
	registerSkillReadHandlers(skillRoutes, skills)
	registerSkillCatalogWriteHandlers(skillRoutes.Group("/catalog"), skills)
}
