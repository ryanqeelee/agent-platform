package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// RegisterTenantMemoryConfigRoutes keeps memory policy updates out of the
// generic whole-tenant update path. The active tenant is authentication scope.
func RegisterTenantMemoryConfigRoutes(
	r *gin.RouterGroup, h *handler.TenantMemoryConfigHandler, g *rbacGuards,
) {
	if h == nil {
		return
	}
	tenantRoutes := r.Group("/tenants")
	g.apiKeyRoute(tenantRoutes, http.MethodGet, "/kv/memory-config",
		apiKeyManageTenantSettings(apiKeyFullAccess()), g.Viewer(), h.Get)
	g.apiKeyRoute(tenantRoutes, http.MethodPut, "/kv/memory-config",
		apiKeyManageTenantSettings(apiKeyFullAccess()), g.Admin(), h.Update)
}
