package router

import (
	"net/http"
	"testing"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
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
		&handler.SandboxConfigHandler{},
		&handler.SandboxSkillHandler{},
		&handler.SkillHandler{},
		&handler.SystemHandler{},
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
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/skills"},
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
