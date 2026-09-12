package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
)

// Models are platform-owned infrastructure. Enterprise members consume only
// configured/enabled capability state through business APIs; concrete model,
// provider, interface, endpoint, and credential metadata stays platform-only.
func RegisterModelRoutes(
	r *gin.RouterGroup,
	handler *handler.ModelHandler,
	credHandler *handler.ModelCredentialsHandler,
	g *rbacGuards,
) {
	models := g.apiKeyGroup(
		r.Group("/models", g.SystemAdmin()),
		apiKeyPlatform(types.APIKeyCapabilityManageModels),
	)
	{
		models.GET("/providers", handler.ListModelProviders)
		models.POST("", handler.CreateModel)
		models.GET("", handler.ListModels)
		models.POST("/:id/debug", handler.DebugModel)
		models.GET("/:id", handler.GetModel)
		models.PUT("/:id", handler.UpdateModel)
		models.DELETE("/:id", handler.DeleteModel)
		models.PUT("/:id/credentials", credHandler.Put)
		models.DELETE("/:id/credentials/:field", credHandler.DeleteField)
	}
}

func registerSandboxConfigHandlers(
	configs *gin.RouterGroup,
	h *handler.SandboxConfigHandler,
	skills *handler.SandboxSkillHandler,
) {
	{
		configs.GET("", h.List)
		configs.PUT("/workspace-policy", h.SetWorkspacePolicy)
		configs.POST("/templates/query", h.QueryTemplates)
		configs.POST("", h.Create)
		configs.GET("/:id", h.Get)
		configs.PUT("/:id", h.Update)
		configs.DELETE("/:id", h.Delete)
		configs.GET("/:id/sandboxes", h.Inventory)
		// Skills are platform-only throughout, reads included: an upload drives a
		// root shell whose output is baked into the image every session of
		// this config boots, and the listing names what that image carries.
		configs.GET("/:id/skills", skills.List)
		configs.POST("/:id/skills", skills.Upload)
		configs.GET("/:id/skills/:skillId", skills.Get)
		configs.GET("/:id/skills/:skillId/files", skills.ListFiles)
		configs.GET("/:id/skills/:skillId/files/content", skills.GetFile)
		configs.POST("/:id/skills/:skillId/reinstall", skills.Reinstall)
		configs.POST("/:id/skills/:skillId/stop", skills.Stop)
		configs.PATCH("/:id/skills/:skillId", skills.Patch)
		configs.DELETE("/:id/skills/:skillId", skills.Delete)
		configs.GET("/:id/skills/:skillId/install-events", skills.InstallEvents)
		configs.GET("/:id/skills/:skillId/transcript", skills.InstallTranscript)
		configs.GET("/:id/skills/:skillId/transcript/history", skills.InstallTranscriptHistory)
	}
}

// RegisterEvaluationRoutes registers evaluation endpoints. Running an
// evaluation drives LLM calls (cost) and reads from KBs across the
// tenant; gate to Admin+ until product asks for a finer-grained
// matrix.
func RegisterEvaluationRoutes(r *gin.RouterGroup, handler *handler.EvaluationHandler, g *rbacGuards) {
	evaluationRoutes := g.apiKeyGroup(r.Group("/evaluation"), apiKeyRunEvaluations(apiKeyFullAccess()))
	{
		evaluationRoutes.POST("", g.Admin(), handler.Evaluation)
		evaluationRoutes.GET("", g.Viewer(), handler.GetEvaluationResult)
	}
}

func RegisterInitializationRoutes(r *gin.RouterGroup, handler *handler.InitializationHandler, g *rbacGuards) {
	// 初始化接口
	// GetCurrentConfigByKB 是只读，Viewer+ 即可（KB 受限 key 可读其范围内的 KB）。
	g.apiKeyRoute(r, http.MethodGet, "/initialization/config/:kbId",
		apiKeyRetrieve(apiKeyFullAccess()), g.Viewer(), g.KBAccessRead("kbId"), handler.GetCurrentConfigByKB)
	// Full initialization binds models and storage, so only the platform may run it.
	g.apiKeyRoute(r, http.MethodPost, "/initialization/initialize/:kbId",
		apiKeyPlatform(types.APIKeyCapabilityManageModels), g.SystemAdmin(), handler.InitializeByKB)
	// Enterprise admins may still maintain business-level chunking through the
	// safe handler projection; model/parser/storage bindings stay platform-owned.
	g.apiKeyRoute(r, http.MethodPut, "/initialization/config/:kbId",
		apiKeyManageKnowledgeBases(apiKeyFullAccess()), g.Admin(), g.KBAccessWrite("kbId"), handler.UpdateKBConfig)

	// Concrete model runtime inspection and mutation belongs to the platform.
	modelPolicy := apiKeyPlatform(types.APIKeyCapabilityManageModels)
	g.apiKeyRoute(r, http.MethodGet, "/initialization/ollama/status", modelPolicy, g.SystemAdmin(), handler.CheckOllamaStatus)
	g.apiKeyRoute(r, http.MethodGet, "/initialization/ollama/models", modelPolicy, g.SystemAdmin(), handler.ListOllamaModels)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/ollama/models/check", modelPolicy, g.SystemAdmin(), handler.CheckOllamaModels)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/ollama/models/download", modelPolicy, g.SystemAdmin(), handler.DownloadOllamaModel)
	g.apiKeyRoute(r, http.MethodGet, "/initialization/ollama/download/progress/:taskId", modelPolicy, g.SystemAdmin(), handler.GetDownloadProgress)
	g.apiKeyRoute(r, http.MethodGet, "/initialization/ollama/download/tasks", modelPolicy, g.SystemAdmin(), handler.ListDownloadTasks)

	// 远程API相关接口
	g.apiKeyRoute(r, http.MethodPost, "/initialization/remote/check", modelPolicy, g.SystemAdmin(), handler.CheckRemoteModel)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/embedding/test", modelPolicy, g.SystemAdmin(), handler.TestEmbeddingModel)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/rerank/check", modelPolicy, g.SystemAdmin(), handler.CheckRerankModel)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/asr/check", modelPolicy, g.SystemAdmin(), handler.CheckASRModel)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/multimodal/test", modelPolicy, g.SystemAdmin(), handler.TestMultimodalFunction)

	g.apiKeyRoute(r, http.MethodPost, "/initialization/extract/text-relation", modelPolicy, g.SystemAdmin(), handler.ExtractTextRelations)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/extract/fabri-tag", modelPolicy, g.SystemAdmin(), handler.FabriTag)
	g.apiKeyRoute(r, http.MethodPost, "/initialization/extract/fabri-text", modelPolicy, g.SystemAdmin(), handler.FabriText)
}

// RegisterMCPServiceRoutes registers MCP service routes.
//
// MCP service configuration is platform infrastructure. Enterprise Admins may
// list the safe service catalog and users may complete their own OAuth flow,
// but neither can inspect endpoints, tools, resources, credentials, or policy.
func RegisterMCPServiceRoutes(
	r *gin.RouterGroup,
	handler *handler.MCPServiceHandler,
	credHandler *handler.MCPCredentialsHandler,
	oauthHandler *handler.MCPOAuthHandler,
	g *rbacGuards,
) {
	// MCP OAuth provider redirect. Registered OUTSIDE the /mcp-services group
	// to avoid a static-vs-":id" route conflict, and left unauthenticated
	// (allow-listed in middleware/auth.go) because the third-party browser
	// redirect carries no WeKnora bearer — the single-use state authenticates.
	r.GET("/mcp-oauth/callback", oauthHandler.Callback)

	mcpServices := g.apiKeyGroup(
		r.Group("/mcp-services"),
		apiKeyPlatform(types.APIKeyCapabilityManageMCPServices),
	)
	{
		mcpServices.POST("", g.SystemAdmin(), handler.CreateMCPService)
		mcpServices.GET("", g.Admin(), handler.ListMCPServices)
		mcpServices.GET("/:id", g.SystemAdmin(), handler.GetMCPService)
		mcpServices.PUT("/:id", g.SystemAdmin(), handler.UpdateMCPService)
		mcpServices.DELETE("/:id", g.SystemAdmin(), handler.DeleteMCPService)
		mcpServices.POST("/:id/test", g.SystemAdmin(), handler.TestMCPService)
		mcpServices.GET("/:id/tools", g.SystemAdmin(), handler.GetMCPServiceTools)
		mcpServices.GET("/:id/resources", g.SystemAdmin(), handler.GetMCPServiceResources)
		mcpServices.PUT("/:id/credentials", g.SystemAdmin(), credHandler.Put)
		mcpServices.DELETE("/:id/credentials/:field", g.SystemAdmin(), credHandler.DeleteField)
		mcpServices.GET("/:id/tool-approvals", g.SystemAdmin(), handler.ListMCPToolApprovals)
		mcpServices.PUT("/:id/tool-approvals/:tool_name", g.SystemAdmin(), handler.SetMCPToolApproval)
		// Per-user OAuth authorization flow. Viewer+ may authorize/inspect/
		// revoke their own token; the callback is the separate public route
		// registered above.
		mcpServices.POST("/:id/oauth/authorize-url", g.Viewer(), oauthHandler.AuthorizeURL)
		mcpServices.GET("/:id/oauth/status", g.Viewer(), oauthHandler.Status)
		mcpServices.DELETE("/:id/oauth/token", g.Viewer(), oauthHandler.Revoke)
	}

	// /agent tool-approval + OAuth resolution are interactive human flows;
	// not declared for API keys (default-deny).
	agentTool := r.Group("/agent")
	{
		// Resolving a pending tool-approval is gated to tenant members
		// (Viewer+). The approval card surfaces inside an agent chat the
		// caller initiated — restricting it to Admin+ blocks the only
		// people who actually have context to approve, so the gate is
		// kept at "anyone in the tenant" instead.
		agentTool.POST("/tool-approvals/:pending_id", g.Viewer(), handler.ResolveToolApproval)
		// Resume an agent run paused on an in-conversation MCP OAuth prompt.
		// Same tenant-member (Viewer+) gating rationale as tool-approvals.
		agentTool.POST("/mcp-oauth-resolutions/:pending_id", g.Viewer(), oauthHandler.ResolveMCPOAuth)
		agentTool.POST("/mcp-oauth-resolutions/:pending_id/cancel", g.Viewer(), oauthHandler.CancelMCPOAuth)
	}
}

// RegisterWebSearchRoutes registers web search routes
func RegisterWebSearchRoutes(r *gin.RouterGroup, webSearchHandler *handler.WebSearchHandler, g *rbacGuards) {
	// Web search providers — Viewer+ (read-only listing of provider catalog).
	webSearch := r.Group("/web-search")
	{
		webSearch.GET("/providers", g.SystemAdmin(), webSearchHandler.GetProviders)
	}
}

// RegisterWebSearchProviderRoutes registers CRUD routes for web search
// provider configurations.
//
// Provider rows are platform infrastructure. Workspace users receive only a
// safe ready/not-ready projection from the list route used by chat.
func RegisterWebSearchProviderRoutes(
	r *gin.RouterGroup,
	h *handler.WebSearchProviderHandler,
	credHandler *handler.WebSearchProviderCredentialsHandler,
	g *rbacGuards,
) {
	providers := g.apiKeyGroup(r.Group("/web-search-providers"), apiKeyManageWebSearch(apiKeyFullAccess()))
	{
		providers.GET("/types", g.SystemAdmin(), h.ListProviderTypes)
		providers.POST("/test", g.SystemAdmin(), h.TestProviderRaw)
		// CRUD
		providers.POST("", g.SystemAdmin(), h.CreateProvider)
		providers.GET("", g.Viewer(), h.ListProviders)
		providers.GET("/:id", g.SystemAdmin(), h.GetProvider)
		providers.PUT("/:id", g.SystemAdmin(), h.UpdateProvider)
		providers.DELETE("/:id", g.SystemAdmin(), h.DeleteProvider)
		providers.PUT("/:id/credentials", g.SystemAdmin(), credHandler.Put)
		providers.DELETE("/:id/credentials/:field", g.SystemAdmin(), credHandler.DeleteField)
		providers.POST("/:id/test", g.SystemAdmin(), h.TestProviderByID)
	}
}

// RegisterVectorStoreRoutes registers CRUD routes for vector store configurations.
//
// Vector stores are tenant-level infrastructure; reads are Viewer+, all
// writes (and connection tests, which probe external systems with stored
// credentials) are Admin+.
func RegisterVectorStoreRoutes(r *gin.RouterGroup, h *handler.VectorStoreHandler, g *rbacGuards) {
	stores := g.apiKeyGroup(
		r.Group("/vector-stores", g.SystemAdmin()),
		apiKeyPlatform(types.APIKeyCapabilityManageVectorStores),
	)
	{
		stores.GET("/types", h.ListStoreTypes)
		stores.POST("/test", h.TestStoreRaw)
		stores.POST("", h.CreateStore)
		stores.GET("", h.ListStores)
		stores.GET("/:id", h.GetStore)
		stores.PUT("/:id", h.UpdateStore)
		stores.DELETE("/:id", h.DeleteStore)
		stores.POST("/:id/test", h.TestStoreByID)
	}
}

// RegisterStorageBackendRoutes manages concrete object/file storage instances.
func RegisterStorageBackendRoutes(r *gin.RouterGroup, h *handler.StorageBackendHandler, g *rbacGuards) {
	backends := g.apiKeyGroup(
		r.Group("/storage-backends", g.SystemAdmin()),
		apiKeyPlatform(types.APIKeyCapabilityManageStorageBackends),
	)
	{
		backends.GET("/types", h.Types)
		backends.POST("/test", h.TestRaw)
		backends.POST("", h.Create)
		backends.GET("", h.List)
		backends.GET("/:id", h.Get)
		backends.PUT("/:id", h.Update)
		backends.DELETE("/:id", h.Delete)
		backends.POST("/:id/test", h.TestByID)
		backends.PUT("/:id/default", h.SetDefault)
	}
}

// RegisterDataSourceRoutes 注册数据源相关的路由
//
// Data sources hold external service credentials (Feishu/Notion/Yuque)
// and trigger sync jobs that mutate KB content tenant-wide. Reads are
// Viewer+; everything else (CRUD, validation, sync control, credential
// subresource) is Admin+.
func RegisterDataSourceRoutes(
	r *gin.RouterGroup,
	handler *handler.DataSourceHandler,
	credHandler *handler.DataSourceCredentialsHandler,
	g *rbacGuards,
) {
	// Data source routes
	ds := g.apiKeyGroup(r.Group("/datasource"), apiKeyManageDataSources(apiKeyFullAccess()))
	{
		// Get available connector types — Viewer+
		ds.GET("/types", g.Viewer(), handler.GetAvailableConnectors)

		// Validate credentials without persistence (for "Test Connection" button) — Admin+
		ds.POST("/validate-credentials", g.Admin(), handler.ValidateCredentials)

		// CRUD operations
		ds.POST("", g.Admin(), handler.CreateDataSource)
		ds.GET("", g.Viewer(), handler.ListDataSources)
		ds.GET("/:id", g.Viewer(), handler.GetDataSource)
		ds.PUT("/:id", g.Admin(), handler.UpdateDataSource)
		ds.DELETE("/:id", g.Admin(), handler.DeleteDataSource)

		// Credential subresource. Single logical field "credentials" because
		// connector credentials are a per-connector atomic map (see
		// internal/handler/datasource_credentials.go). — Admin+
		ds.PUT("/:id/credentials", g.Admin(), credHandler.Put)
		ds.DELETE("/:id/credentials/:field", g.Admin(), credHandler.DeleteField)

		// Connection and resource management — Admin+
		ds.POST("/:id/validate", g.Admin(), handler.ValidateConnection)
		ds.GET("/:id/resources", g.Admin(), handler.ListAvailableResources)
		ds.POST("/:id/resource-ancestors", g.Admin(), handler.ResolveResourceAncestors)

		// Sync management — Admin+
		ds.POST("/:id/sync", g.Admin(), handler.ManualSync)
		ds.POST("/:id/pause", g.Admin(), handler.PauseDataSource)
		ds.POST("/:id/resume", g.Admin(), handler.ResumeDataSource)

		// Sync logs — Viewer+ (read-only audit trail)
		ds.GET("/:id/logs", g.Viewer(), handler.GetSyncLogs)
		ds.GET("/logs/:log_id", g.Viewer(), handler.GetSyncLog)
	}
}

// RegisterWeKnoraCloudRoutes 注册 WeKnoraCloud 初始化路由
// RegisterWeKnoraCloudRoutes registers the WeKnoraCloud credential
// management endpoints. SaveCredentials persists external SaaS keys
// for the tenant (Admin+), Status is a low-risk readiness probe (Viewer+).
func RegisterWeKnoraCloudRoutes(r *gin.RouterGroup, handler *handler.WeKnoraCloudHandler, g *rbacGuards) {
	policy := apiKeyPlatform(types.APIKeyCapabilityManageModels)
	g.apiKeyRoute(r, http.MethodPost, "/weknoracloud/credentials", policy, g.SystemAdmin(), handler.SaveCredentials)
	g.apiKeyRoute(r, http.MethodGet, "/models/weknoracloud/status", policy, g.SystemAdmin(), handler.Status)
}
