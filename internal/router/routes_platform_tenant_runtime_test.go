package router

import (
	"net/http"
	"testing"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/gin-gonic/gin"
)

func TestSystemAdminRuntimeRouteSurfacesAndAPIKeyDefaultDeny(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	guards := &rbacGuards{apiKeyAuthorizer: middleware.NewAPIKeyRouteAuthorizer()}
	RegisterSystemAdminTenantRuntimeRoutes(
		v1,
		nil,
		&handler.AICapabilityPlanHandler{},
		guards,
	)
	RegisterPlatformMemoryRuntimeConfigRoutes(
		v1,
		&handler.PlatformMemoryRuntimeConfigHandler{},
		guards,
	)
	RegisterSystemRoutes(v1, &handler.SystemHandler{}, guards)
	RegisterSystemAdminSandboxSkillRoutes(
		v1,
		&handler.SandboxConfigHandler{},
		&handler.SandboxSkillHandler{},
		&handler.SkillHandler{},
		&handler.SystemHandler{},
		guards,
	)
	RegisterSandboxPermissionRoutes(v1, &handler.SandboxConfigHandler{}, guards)

	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	want := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/system/admin/memory-runtime-config"},
		{http.MethodPut, "/api/v1/system/admin/memory-runtime-config"},
		{http.MethodGet, "/api/v1/system/admin/tenants/:tenant_id/retrieval-processing-settings"},
		{http.MethodGet, "/api/v1/system/admin/sandbox-configs"},
		{http.MethodPut, "/api/v1/system/admin/sandbox-configs/default"},
		{http.MethodPost, "/api/v1/system/admin/sandbox-configs"},
		{http.MethodPost, "/api/v1/system/admin/sandbox-configs/check"},
		{http.MethodPost, "/api/v1/system/admin/sandbox-configs/templates/query"},
		{http.MethodGet, "/api/v1/system/admin/sandbox-configs/:id"},
		{http.MethodPut, "/api/v1/system/admin/sandbox-configs/:id"},
		{http.MethodDelete, "/api/v1/system/admin/sandbox-configs/:id"},
		{http.MethodGet, "/api/v1/system/admin/sandbox-configs/:id/sandboxes"},
		{http.MethodGet, "/api/v1/system/admin/sandbox-configs/:id/skills"},
		{http.MethodPost, "/api/v1/system/admin/sandbox-configs/:id/skills"},
		{http.MethodGet, "/api/v1/system/admin/sandbox-configs/:id/skills/:skillId"},
		{http.MethodGet, "/api/v1/system/admin/sandbox-configs/:id/skills/:skillId/files"},
		{http.MethodGet, "/api/v1/system/admin/sandbox-configs/:id/skills/:skillId/files/content"},
		{http.MethodPost, "/api/v1/system/admin/sandbox-configs/:id/skills/:skillId/reinstall"},
		{http.MethodPost, "/api/v1/system/admin/sandbox-configs/:id/skills/:skillId/stop"},
		{http.MethodPatch, "/api/v1/system/admin/sandbox-configs/:id/skills/:skillId"},
		{http.MethodDelete, "/api/v1/system/admin/sandbox-configs/:id/skills/:skillId"},
		{http.MethodGet, "/api/v1/system/admin/sandbox-configs/:id/skills/:skillId/install-events"},
		{http.MethodGet, "/api/v1/system/admin/sandbox-configs/:id/skills/:skillId/transcript"},
		{http.MethodGet, "/api/v1/system/admin/sandbox-configs/:id/skills/:skillId/transcript/history"},
		{http.MethodGet, "/api/v1/system/admin/skills"},
		{http.MethodPost, "/api/v1/system/admin/skills"},
		{http.MethodPost, "/api/v1/system/admin/skills/:id/install"},
		{http.MethodGet, "/api/v1/system/admin/skills/:id/files"},
		{http.MethodGet, "/api/v1/system/admin/skills/:id/files/content"},
		{http.MethodDelete, "/api/v1/system/admin/skills/:id"},
		{http.MethodGet, "/api/v1/sandbox-policy"},
		{http.MethodPut, "/api/v1/sandbox-policy"},
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
		http.MethodGet + " /api/v1/system/admin/tenants/:tenant_id/parser-engine-config",
		http.MethodPut + " /api/v1/system/admin/tenants/:tenant_id/parser-engine-config",
		http.MethodGet + " /api/v1/system/admin/tenants/:tenant_id/memory-config",
		http.MethodPut + " /api/v1/system/admin/tenants/:tenant_id/memory-config",
		http.MethodGet + " /api/v1/system/admin/tenants/:tenant_id/sandbox-configs",
		http.MethodGet + " /api/v1/system/admin/tenants/:tenant_id/skills/catalog",
	} {
		if _, ok := routes[oldRoute]; ok {
			t.Errorf("legacy platform route is still registered: %s", oldRoute)
		}
	}
}
