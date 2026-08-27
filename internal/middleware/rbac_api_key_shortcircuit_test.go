package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func apiKeyRBACHarness(scope types.TenantAPIKeyScope, role types.TenantRole, mw gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, role)
		ctx = types.WithTenantAPIKeyScope(ctx, scope)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/protected", mw, func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))
	return w
}

func TestRequireRole_ShortCircuitsAPIKey(t *testing.T) {
	w := apiKeyRBACHarness(types.TenantAPIKeyScope{}, types.TenantRoleViewer,
		RequireRole(types.TenantRoleAdmin, cfgRBAC(true)))
	if w.Code != http.StatusOK {
		t.Fatalf("API-key principal should short-circuit RequireRole, got %d", w.Code)
	}
}

func TestRequireSystemAdmin_RejectsWorkspaceAPIKey(t *testing.T) {
	w := apiKeyRBACHarness(types.TenantAPIKeyScope{FullAccess: true}, types.TenantRoleOwner,
		RequireSystemAdmin(cfgRBAC(true)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("workspace API key must be rejected, got %d", w.Code)
	}
}

func TestRequireSystemAdmin_AllowsPlatformAPIKeyAfterRouteGate(t *testing.T) {
	w := apiKeyRBACHarness(types.TenantAPIKeyScope{ScopeType: types.APIKeyScopePlatform}, types.TenantRoleViewer,
		RequireSystemAdmin(cfgRBAC(true)))
	if w.Code != http.StatusOK {
		t.Fatalf("platform API key should pass after route authorization, got %d", w.Code)
	}
}

func TestRequireRole_JWTViewerStillDenied(t *testing.T) {
	w := rbacTestHarness(types.TenantRoleViewer, "u1",
		RequireRole(types.TenantRoleAdmin, &config.Config{Tenant: &config.TenantConfig{EnableRBAC: boolPtr(true)}}))
	if w.Code != http.StatusForbidden {
		t.Fatalf("JWT Viewer must still be denied by Admin gate, got %d", w.Code)
	}
}

func boolPtr(value bool) *bool { return &value }
