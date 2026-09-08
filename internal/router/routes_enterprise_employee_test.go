package router

import (
	"context"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestEnterpriseEmployeeRouteGuards(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		role               types.TenantRole
	}{
		{"viewer-create", "POST", "/api/v1/tenants/1/employees", types.TenantRoleViewer},
		{"foreign-create", "POST", "/api/v1/tenants/2/employees", types.TenantRoleAdmin},
		{"admin-delete", "DELETE", "/api/v1/tenants/1", types.TenantRoleAdmin},
		{"viewer-delete", "DELETE", "/api/v1/tenants/1", types.TenantRoleViewer},
	} {
		t.Run(tc.name, func(t *testing.T) {
			enforce := true
			g := &rbacGuards{cfg: &config.Config{Tenant: &config.TenantConfig{EnableRBAC: &enforce}}}
			r := gin.New()
			r.Use(middleware.ErrorHandler())
			r.Use(func(c *gin.Context) {
				ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, tc.role)
				ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(1))
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			RegisterTenantRoutes(r.Group("/api/v1"), &handler.TenantHandler{}, &handler.TenantMemberHandler{}, nil, nil, g)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != 403 {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
