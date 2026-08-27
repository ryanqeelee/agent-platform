package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func TestViewerCannotReachRegisteredEmployeeWorkspaceMutationRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name     string
		method   string
		path     string
		register func(*gin.RouterGroup, *rbacGuards)
	}{
		{
			name: "model create", method: http.MethodPost, path: "/api/v1/models",
			register: func(v1 *gin.RouterGroup, g *rbacGuards) {
				RegisterModelRoutes(v1, &handler.ModelHandler{}, &handler.ModelCredentialsHandler{}, g)
			},
		},
		{
			name: "knowledge base create", method: http.MethodPost, path: "/api/v1/knowledge-bases",
			register: func(v1 *gin.RouterGroup, g *rbacGuards) {
				RegisterKnowledgeBaseRoutes(v1, &handler.KnowledgeBaseHandler{}, g)
			},
		},
		{
			name: "agent create", method: http.MethodPost, path: "/api/v1/agents",
			register: func(v1 *gin.RouterGroup, g *rbacGuards) {
				RegisterCustomAgentRoutes(v1, &handler.CustomAgentHandler{}, g)
			},
		},
		{
			name: "agent update", method: http.MethodPut, path: "/api/v1/agents/agent-1",
			register: func(v1 *gin.RouterGroup, g *rbacGuards) {
				RegisterCustomAgentRoutes(v1, &handler.CustomAgentHandler{}, g)
			},
		},
		{
			name: "agent delete", method: http.MethodDelete, path: "/api/v1/agents/agent-1",
			register: func(v1 *gin.RouterGroup, g *rbacGuards) {
				RegisterCustomAgentRoutes(v1, &handler.CustomAgentHandler{}, g)
			},
		},
		{
			name: "agent copy", method: http.MethodPost, path: "/api/v1/agents/agent-1/copy",
			register: func(v1 *gin.RouterGroup, g *rbacGuards) {
				RegisterCustomAgentRoutes(v1, &handler.CustomAgentHandler{}, g)
			},
		},
		{
			name: "agent share create", method: http.MethodPost, path: "/api/v1/agents/agent-1/shares",
			register: func(v1 *gin.RouterGroup, g *rbacGuards) {
				RegisterOrganizationRoutes(v1, &handler.OrganizationHandler{}, g)
			},
		},
		{
			name: "agent share list", method: http.MethodGet, path: "/api/v1/agents/agent-1/shares",
			register: func(v1 *gin.RouterGroup, g *rbacGuards) {
				RegisterOrganizationRoutes(v1, &handler.OrganizationHandler{}, g)
			},
		},
		{
			name: "agent share delete", method: http.MethodDelete, path: "/api/v1/agents/agent-1/shares/share-1",
			register: func(v1 *gin.RouterGroup, g *rbacGuards) {
				RegisterOrganizationRoutes(v1, &handler.OrganizationHandler{}, g)
			},
		},
		{
			name: "organization create", method: http.MethodPost, path: "/api/v1/organizations",
			register: func(v1 *gin.RouterGroup, g *rbacGuards) {
				RegisterOrganizationRoutes(v1, &handler.OrganizationHandler{}, g)
			},
		},
		{
			name: "tenant create", method: http.MethodPost, path: "/api/v1/tenants",
			register: func(v1 *gin.RouterGroup, g *rbacGuards) {
				RegisterTenantRoutes(v1, &handler.TenantHandler{}, nil, nil, nil, g)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			enforce := true
			g := &rbacGuards{cfg: &config.Config{Tenant: &config.TenantConfig{EnableRBAC: &enforce}}}
			r := gin.New()
			r.Use(func(c *gin.Context) {
				ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, types.TenantRoleViewer)
				ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(1))
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			tc.register(r.Group("/api/v1"), g)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != http.StatusForbidden {
				t.Fatalf("%s %s status = %d, want %d; body=%s", tc.method, tc.path, w.Code, http.StatusForbidden, w.Body.String())
			}
		})
	}
}

func TestTenantCreatorAllowsTenantlessOnboarding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	enforce := true
	g := &rbacGuards{cfg: &config.Config{Tenant: &config.TenantConfig{EnableRBAC: &enforce}}}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, types.TenantRoleViewer)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	called := false
	r.POST("/api/v1/tenants", g.TenantCreator(), func(c *gin.Context) {
		called = true
		c.Status(http.StatusCreated)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/tenants", nil))
	if w.Code != http.StatusCreated || !called {
		t.Fatalf("tenantless onboarding status=%d called=%t, want status=%d called=true", w.Code, called, http.StatusCreated)
	}
}
