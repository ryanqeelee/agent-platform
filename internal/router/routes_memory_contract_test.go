package router

import (
	"net/http"
	"testing"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPersonalMemoryContractRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	guards := &rbacGuards{}
	RegisterMemoryRoutes(v1, &handler.MemoryHandler{}, guards)
	RegisterTenantMemoryConfigRoutes(v1, &handler.TenantMemoryConfigHandler{}, guards)

	routes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, route := range []string{
		http.MethodGet + " /api/v1/memory/snapshot",
		http.MethodPost + " /api/v1/memory/commands",
		http.MethodGet + " /api/v1/memory/commands/:operation_id",
		http.MethodPost + " /api/v1/memory/expressions",
		http.MethodGet + " /api/v1/tenants/kv/memory-config",
		http.MethodPut + " /api/v1/tenants/kv/memory-config",
	} {
		require.Contains(t, routes, route)
	}

	guards.assertAPIKeyPoliciesMatchRoutes(engine)
}
