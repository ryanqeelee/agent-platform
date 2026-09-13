package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

func TestSystemAdminTenantRuntimeRouteSurfaceAndAPIKeyDefaultDeny(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	guards := &rbacGuards{apiKeyAuthorizer: middleware.NewAPIKeyRouteAuthorizer()}
	RegisterSystemAdminTenantRuntimeRoutes(
		v1,
		nil,
		&handler.TenantHandler{},
		&handler.MessageHandler{},
		&handler.SandboxConfigHandler{},
		&handler.SandboxSkillHandler{},
		&handler.SkillHandler{},
		&handler.CustomAgentHandler{},
		&handler.SystemHandler{},
		&handler.TenantMemoryConfigHandler{},
		&handler.AICapabilityPlanHandler{},
		&handler.MCPServiceHandler{},
		&handler.MCPCredentialsHandler{},
		&handler.WebSearchProviderHandler{},
		&handler.WebSearchProviderCredentialsHandler{},
		&handler.VectorStoreHandler{},
		&handler.StorageBackendHandler{},
		guards,
	)
	RegisterSystemRoutes(v1, &handler.SystemHandler{}, guards)

	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	want := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/chat-history-config"},
		{http.MethodPut, "/api/v1/system/admin/tenants/:tenant_id/chat-history-config"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/chat-history-stats"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/memory-config"},
		{http.MethodPut, "/api/v1/system/admin/tenants/:tenant_id/memory-config"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/parser-engine-config"},
		{http.MethodPut, "/api/v1/system/admin/tenants/:tenant_id/parser-engine-config"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/retrieval-processing-settings"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/web-search-providers"},
		{http.MethodPost, "/api/v1/system/admin/tenants/:tenant_id/web-search-providers"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/vector-stores"},
		{http.MethodPost, "/api/v1/system/admin/tenants/:tenant_id/vector-stores"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/storage-backends"},
		{http.MethodPost, "/api/v1/system/admin/tenants/:tenant_id/storage-backends"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/mcp-services"},
		{http.MethodPost, "/api/v1/system/admin/tenants/:tenant_id/mcp-services"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs"},
		{http.MethodPut, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/workspace-policy"},
		{http.MethodPost, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs"},
		{http.MethodPost, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/check"},
		{http.MethodPost, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/templates/query"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id"},
		{http.MethodPut, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id"},
		{http.MethodDelete, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/sandboxes"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/skills"},
		{http.MethodPost, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/skills"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/skills/:skillId"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/skills/:skillId/files"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/skills/:skillId/files/content"},
		{http.MethodPost, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/skills/:skillId/reinstall"},
		{http.MethodPost, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/skills/:skillId/stop"},
		{http.MethodPatch, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/skills/:skillId"},
		{http.MethodDelete, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/skills/:skillId"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/skills/:skillId/install-events"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/skills/:skillId/transcript"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/sandbox-configs/:id/skills/:skillId/transcript/history"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/skills"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/skills/installer-agent"},
		{http.MethodPut, "/api/v1/system/admin/tenants/:tenant_id/skills/installer-agent"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/skills/catalog"},
		{http.MethodPost, "/api/v1/system/admin/tenants/:tenant_id/skills/catalog"},
		{http.MethodPost, "/api/v1/system/admin/tenants/:tenant_id/skills/catalog/:id/install"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/skills/catalog/:id/files"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/skills/catalog/:id/files/content"},
		{http.MethodDelete, "/api/v1/system/admin/tenants/:tenant_id/skills/catalog/:id"},
	}
	for _, route := range want {
		key := route.method + " " + route.path
		if _, ok := routes[key]; !ok {
			t.Errorf("missing route %s", key)
		}
		if _, ok := guards.apiKeyAuthorizer.Lookup(route.method, route.path); ok {
			t.Errorf("platform API key policy unexpectedly declared for %s", key)
		}
	}
	for _, oldRoute := range []string{
		http.MethodGet + " /api/v1/sandbox-configs",
		http.MethodGet + " /api/v1/skills",
		http.MethodGet + " /api/v1/skills/catalog",
		http.MethodPost + " /api/v1/skills/catalog",
		http.MethodPost + " /api/v1/system/sandbox-check",
	} {
		if _, ok := routes[oldRoute]; ok {
			t.Errorf("legacy platform route is still registered: %s", oldRoute)
		}
	}
}

type chatHistoryTenantService struct {
	interfaces.TenantService
	tenant         *types.Tenant
	updated        *types.Tenant
	principalScope uint64
}

func (s *chatHistoryTenantService) GetTenantByID(_ context.Context, id uint64) (*types.Tenant, error) {
	if s.tenant != nil && s.tenant.ID == id {
		return s.tenant, nil
	}
	return nil, nil
}

func (s *chatHistoryTenantService) UpdateTenant(ctx context.Context, tenant *types.Tenant) (*types.Tenant, error) {
	if user, ok := ctx.Value(types.UserContextKey).(*types.User); ok {
		s.principalScope = user.TenantID
	}
	s.updated = tenant
	return tenant, nil
}

type chatHistoryKBService struct {
	interfaces.KnowledgeBaseService
	created        *types.KnowledgeBase
	requestTenant  uint64
	principalScope uint64
}

func (s *chatHistoryKBService) CreateKnowledgeBase(ctx context.Context, kb *types.KnowledgeBase) (*types.KnowledgeBase, error) {
	s.requestTenant, _ = types.TenantIDFromContext(ctx)
	if user, ok := ctx.Value(types.UserContextKey).(*types.User); ok {
		s.principalScope = user.TenantID
	}
	s.created = kb
	kb.ID = "chat-history-kb"
	return kb, nil
}

func TestSystemAdminTenantRuntimeEnablesChatHistoryForTargetTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantService := &chatHistoryTenantService{tenant: &types.Tenant{ID: 42, Name: "target"}}
	kbService := &chatHistoryKBService{}
	tenantHandler := handler.NewTenantHandler(tenantService, nil, nil, nil, kbService, nil, nil, nil)
	platformUser := &types.User{ID: "platform-admin", IsActive: true, IsSystemAdmin: true}

	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.SystemAdminContextKey, true)
		ctx = context.WithValue(ctx, types.UserContextKey, platformUser)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	RegisterSystemAdminTenantRuntimeRoutes(
		router.Group("/api/v1"),
		tenantService,
		tenantHandler,
		&handler.MessageHandler{},
		&handler.SandboxConfigHandler{},
		&handler.SandboxSkillHandler{},
		&handler.SkillHandler{},
		&handler.CustomAgentHandler{},
		&handler.SystemHandler{},
		nil, nil, nil, nil, nil, nil, nil, nil,
		&rbacGuards{},
	)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/system/admin/tenants/42/chat-history-config",
		strings.NewReader(`{"enabled":true,"embedding_model_id":"embedding-1"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if kbService.requestTenant != 42 || tenantService.updated == nil || tenantService.updated.ID != 42 {
		t.Fatalf("write escaped target tenant: kb tenant=%d updated=%+v", kbService.requestTenant, tenantService.updated)
	}
	if kbService.created == nil || kbService.created.Name != "__chat_history__" || !kbService.created.IsTemporary {
		t.Fatalf("hidden chat-history knowledge base was not created: %+v", kbService.created)
	}
	config := tenantService.updated.ChatHistoryConfig
	if config == nil || !config.Enabled || config.EmbeddingModelID != "embedding-1" || config.KnowledgeBaseID != "chat-history-kb" {
		t.Fatalf("chat-history config was not persisted from the created knowledge base: %+v", config)
	}
	if platformUser.TenantID != 0 || tenantService.principalScope != 0 || kbService.principalScope != 0 {
		t.Fatalf("platform principal was rebound to tenant: user=%d update=%d kb=%d", platformUser.TenantID, tenantService.principalScope, kbService.principalScope)
	}
}

func TestSystemAdminTenantRuntimeChatHistoryRejectsMissingAuthorityOrTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		path        string
		systemAdmin bool
		wantStatus  int
	}{
		{name: "ordinary user", path: "/api/v1/system/admin/tenants/42/chat-history-config", wantStatus: http.StatusForbidden},
		{name: "missing tenant path", path: "/api/v1/system/admin/chat-history-config", systemAdmin: true, wantStatus: http.StatusNotFound},
		{name: "invalid tenant", path: "/api/v1/system/admin/tenants/not-a-tenant/chat-history-stats", systemAdmin: true, wantStatus: http.StatusBadRequest},
		{name: "zero tenant", path: "/api/v1/system/admin/tenants/0/chat-history-config", systemAdmin: true, wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(middleware.ErrorHandler())
			router.Use(func(c *gin.Context) {
				ctx := context.WithValue(c.Request.Context(), types.SystemAdminContextKey, test.systemAdmin)
				ctx = context.WithValue(ctx, types.UserContextKey, &types.User{ID: "caller", IsActive: true, IsSystemAdmin: test.systemAdmin})
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			v1 := router.Group("/api/v1")
			RegisterSystemAdminTenantRuntimeRoutes(
				v1,
				nil,
				&handler.TenantHandler{},
				&handler.MessageHandler{},
				&handler.SandboxConfigHandler{},
				&handler.SandboxSkillHandler{},
				&handler.SkillHandler{},
				&handler.CustomAgentHandler{},
				&handler.SystemHandler{},
				nil, nil, nil, nil, nil, nil, nil, nil,
				&rbacGuards{},
			)

			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}
