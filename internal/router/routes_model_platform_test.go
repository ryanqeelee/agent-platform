package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func TestPlatformInfrastructureRoutesRequireSystemAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
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
	RegisterMCPServiceRoutes(v1, &handler.MCPServiceHandler{}, &handler.MCPCredentialsHandler{}, &handler.MCPOAuthHandler{}, g)
	RegisterVectorStoreRoutes(v1, &handler.VectorStoreHandler{}, g)
	RegisterStorageBackendRoutes(v1, &handler.StorageBackendHandler{}, g)
	RegisterWeKnoraCloudRoutes(v1, &handler.WeKnoraCloudHandler{}, g)
	RegisterSystemRoutes(v1, &handler.SystemHandler{}, g)
	RegisterWebSearchRoutes(v1, &handler.WebSearchHandler{}, g)
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
		{http.MethodGet, "/api/v1/mcp-services"},
		{http.MethodGet, "/api/v1/vector-stores/types"},
		{http.MethodGet, "/api/v1/storage-backends/types"},
		{http.MethodGet, "/api/v1/models/weknoracloud/status"},
		{http.MethodGet, "/api/v1/system/parser-engines"},
		{http.MethodGet, "/api/v1/system/storage-engine-status"},
		{http.MethodGet, "/api/v1/web-search/providers"},
		{http.MethodGet, "/api/v1/skills"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != http.StatusForbidden {
			t.Fatalf("enterprise Owner %s %s status = %d, want %d", tc.method, tc.path, w.Code, http.StatusForbidden)
		}
	}

	allowed := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/models/providers", nil)
	req.Header.Set("X-Test-System-Admin", "true")
	r.ServeHTTP(allowed, req)
	if allowed.Code != http.StatusOK {
		t.Fatalf("system admin status = %d, want %d", allowed.Code, http.StatusOK)
	}
}
