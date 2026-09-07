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

func rbacTestHarness(role types.TenantRole, userID string, mw gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, role)
		ctx = context.WithValue(ctx, types.UserIDContextKey, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/protected", mw, func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))
	return w
}

func cfgRBAC(enabled bool) *config.Config {
	return &config.Config{Tenant: &config.TenantConfig{EnableRBAC: &enabled}}
}

func cfgRBACWithCrossTenant(enabled bool) *config.Config {
	return &config.Config{Tenant: &config.TenantConfig{
		EnableRBAC:              &enabled,
		EnableCrossTenantAccess: true,
	}}
}

func TestRequireRole_AllowsAtMin(t *testing.T) {
	w := rbacTestHarness(types.TenantRoleAdmin, "u1", RequireRole(types.TenantRoleAdmin, cfgRBAC(true)))
	if w.Code != http.StatusOK {
		t.Fatalf("Admin should clear Admin gate, got %d", w.Code)
	}
}

func TestRequireRole_AllowsAboveMin(t *testing.T) {
	w := rbacTestHarness(types.TenantRoleAdmin, "u1", RequireRole(types.TenantRoleAdmin, cfgRBAC(true)))
	if w.Code != http.StatusOK {
		t.Fatalf("Owner should clear Admin gate, got %d", w.Code)
	}
}

func TestRequireRole_RejectsBelowMin(t *testing.T) {
	w := rbacTestHarness(types.TenantRoleContributor, "u1", RequireRole(types.TenantRoleAdmin, cfgRBAC(true)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("Contributor must not clear Admin gate, got %d", w.Code)
	}
}

func TestRequireRole_FailOpenWhenRBACDisabled(t *testing.T) {
	w := rbacTestHarness(types.TenantRoleViewer, "u1", RequireRole(types.TenantRoleOwner, cfgRBAC(false)))
	if w.Code != http.StatusOK {
		t.Fatalf("disabled RBAC must let the request through, got %d", w.Code)
	}
}

func TestRequireRole_CrossTenantSuperuserBypass(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, types.TenantRoleViewer)
		ctx = context.WithValue(ctx, types.UserContextKey, &types.User{ID: "su1", CanAccessAllTenants: true})
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/protected", RequireRole(types.TenantRoleOwner, cfgRBACWithCrossTenant(true)), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("superuser must bypass Owner gate, got %d", w.Code)
	}
}
