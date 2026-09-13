package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type routeMCPServiceStub struct{ interfaces.MCPServiceService }

func (routeMCPServiceStub) ListMCPServices(context.Context) ([]*types.MCPService, error) {
	return []*types.MCPService{}, nil
}

func TestPlatformInfrastructureRoutesRequireSystemAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, types.TenantRoleOwner)
		if c.GetHeader("X-Test-System-Admin") == "true" {
			ctx = context.WithValue(ctx, types.SystemAdminContextKey, true)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	v1 := r.Group("/api/v1")
	g := &rbacGuards{}
	RegisterModelRoutes(v1, &handler.ModelHandler{}, &handler.ModelCredentialsHandler{}, g)
	RegisterCapabilityPlanRoutes(v1, &handler.AICapabilityPlanHandler{}, g)
	RegisterInitializationRoutes(v1, &handler.InitializationHandler{}, g)
	RegisterMCPServiceRoutes(
		v1,
		handler.NewMCPServiceHandler(routeMCPServiceStub{}, nil, nil),
		&handler.MCPCredentialsHandler{},
		&handler.MCPOAuthHandler{},
		g,
	)
	RegisterVectorStoreRoutes(v1, &handler.VectorStoreHandler{}, g)
	RegisterStorageBackendRoutes(v1, &handler.StorageBackendHandler{}, g)
	RegisterWeKnoraCloudRoutes(v1, &handler.WeKnoraCloudHandler{}, g)
	RegisterSystemRoutes(v1, &handler.SystemHandler{}, g)
	RegisterSystemAdminRoutes(v1, &handler.SystemHandler{}, nil, nil, g)
	RegisterWebSearchRoutes(v1, &handler.WebSearchHandler{}, g)
	RegisterWebSearchProviderRoutes(v1, &handler.WebSearchProviderHandler{}, &handler.WebSearchProviderCredentialsHandler{}, g)
	RegisterSkillRoutes(v1, &handler.SkillHandler{}, g)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/models"},
		{http.MethodGet, "/api/v1/models/providers"},
		{http.MethodGet, "/api/v1/models/model-1"},
		{http.MethodGet, "/api/v1/platform/model-runtime-settings"},
		{http.MethodGet, "/api/v1/platform/retrieval-processing-settings"},
		{http.MethodPost, "/api/v1/initialization/initialize/kb-1"},
		{http.MethodGet, "/api/v1/initialization/ollama/models"},
		{http.MethodGet, "/api/v1/system/admin/vector-stores/types"},
		{http.MethodGet, "/api/v1/system/admin/storage-backends/types"},
		{http.MethodGet, "/api/v1/models/weknoracloud/status"},
		{http.MethodGet, "/api/v1/system/admin/parser-engine-config"},
		{http.MethodGet, "/api/v1/system/admin/capabilities"},
		{http.MethodGet, "/api/v1/web-search/providers"},
		{http.MethodGet, "/api/v1/system/admin/mcp-services"},
		{http.MethodGet, "/api/v1/system/admin/web-search-providers"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != http.StatusForbidden {
			t.Fatalf("enterprise Owner %s %s status = %d, want %d", tc.method, tc.path, w.Code, http.StatusForbidden)
		}
	}

	enterpriseCatalog := httptest.NewRecorder()
	r.ServeHTTP(enterpriseCatalog, httptest.NewRequest(http.MethodGet, "/api/v1/mcp-services", nil))
	if enterpriseCatalog.Code != http.StatusOK {
		t.Fatalf("enterprise Owner safe MCP catalog status = %d, want %d", enterpriseCatalog.Code, http.StatusOK)
	}
	for _, path := range []string{"/api/v1/vector-stores", "/api/v1/storage-backends"} {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code == http.StatusForbidden {
			t.Fatalf("enterprise Viewer must reach safe connection capabilities at %s", path)
		}
	}

	allowed := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/models/providers", nil)
	req.Header.Set("X-Test-System-Admin", "true")
	r.ServeHTTP(allowed, req)
	if allowed.Code != http.StatusOK {
		t.Fatalf("system admin status = %d, want %d", allowed.Code, http.StatusOK)
	}

	adminCapabilities := httptest.NewRecorder()
	adminCapabilitiesRequest := httptest.NewRequest(http.MethodGet, "/api/v1/system/admin/capabilities", nil)
	adminCapabilitiesRequest.Header.Set("X-Test-System-Admin", "true")
	r.ServeHTTP(adminCapabilities, adminCapabilitiesRequest)
	if adminCapabilities.Code != http.StatusOK {
		t.Fatalf("system admin capabilities status = %d, want %d", adminCapabilities.Code, http.StatusOK)
	}
	if _, ok := g.apiKeyAuthorizer.Lookup(http.MethodGet, "/api/v1/system/admin/capabilities"); ok {
		t.Fatal("system admin capabilities must remain default-denied for API keys")
	}
}
