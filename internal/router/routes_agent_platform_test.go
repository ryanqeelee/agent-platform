package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func TestAgentAuthoringRequiresPlatformAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, enforce := range []bool{false, true} {
		for _, role := range []types.TenantRole{types.TenantRoleViewer, types.TenantRoleContributor, types.TenantRoleAdmin, types.TenantRoleOwner} {
			r := gin.New()
			r.Use(middleware.ErrorHandler())
			r.Use(func(c *gin.Context) {
				ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, role)
				ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(1))
				ctx = context.WithValue(ctx, types.SystemAdminContextKey, c.GetHeader("X-Test-System-Admin") == "true")
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			g := &rbacGuards{cfg: &config.Config{Tenant: &config.TenantConfig{EnableRBAC: &enforce}}}
			RegisterCustomAgentRoutes(r.Group("/api/v1"), &handler.CustomAgentHandler{}, g)
			for _, route := range []struct{ method, path string }{
				{http.MethodPost, "/api/v1/agents"},
				{http.MethodPut, "/api/v1/agents/agent-1"},
				{http.MethodDelete, "/api/v1/agents/agent-1"},
				{http.MethodPost, "/api/v1/agents/agent-1/copy"},
			} {
				w := httptest.NewRecorder()
				r.ServeHTTP(w, httptest.NewRequest(route.method, route.path, nil))
				if w.Code != http.StatusForbidden {
					t.Fatalf("RBAC=%v role=%s %s %s: status=%d", enforce, role, route.method, route.path, w.Code)
				}
			}
			// A platform administrator passes the guard and reaches body validation.
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", nil)
			req.Header.Set("X-Test-System-Admin", "true")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("platform administrator must reach validation, got %d", w.Code)
			}
		}
	}
}
