package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type platformAgentHTTPRepo struct {
	interfaces.CustomAgentRepository
	row *types.CustomAgent
}

func (r *platformAgentHTTPRepo) GetAgentByID(_ context.Context, id string, tenantID uint64) (*types.CustomAgent, error) {
	if r.row == nil || r.row.ID != id || tenantID != 0 {
		return nil, repository.ErrCustomAgentNotFound
	}
	copy := *r.row
	return &copy, nil
}
func (r *platformAgentHTTPRepo) CreateAgent(_ context.Context, agent *types.CustomAgent) error {
	copy := *agent
	r.row = &copy
	return nil
}
func (r *platformAgentHTTPRepo) UpdateAgent(_ context.Context, agent *types.CustomAgent) error {
	copy := *agent
	r.row = &copy
	return nil
}

type platformAgentHTTPModels struct{ interfaces.ModelRepository }

func (platformAgentHTTPModels) GetByID(context.Context, uint64, string) (*types.Model, error) {
	return nil, nil
}

func TestPlatformAgentRoutesRequireSystemAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := &rbacGuards{cfg: &config.Config{}, apiKeyAuthorizer: middleware.NewAPIKeyRouteAuthorizer()}
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, types.TenantRoleOwner)
		if c.GetHeader("X-Test-API-Key") == "true" {
			ctx = types.WithTenantAPIKeyScope(ctx, types.TenantAPIKeyScope{ScopeType: types.APIKeyScopePlatform, FullAccess: true})
		} else if c.GetHeader("X-Test-System-Admin") == "true" {
			ctx = context.WithValue(ctx, types.SystemAdminContextKey, true)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.Use(g.apiKeyAuthorizer.Middleware())
	h := &handler.PlatformAgentHandler{}
	RegisterPlatformAgentRoutes(r.Group("/api/v1"), h, g)

	for _, path := range []string{
		"/api/v1/system/admin/agents",
		"/api/v1/system/admin/agents/builtin-quick-answer",
		"/api/v1/system/admin/agents/builtin-skill-installer",
		"/api/v1/system/admin/agents/type-presets",
		"/api/v1/system/admin/agents/placeholders",
		"/api/v1/system/admin/agents/prompt-templates",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusForbidden, w.Code, path)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/admin/agents/builtin-skill-installer", bytes.NewBufferString(`{"config":{"agent_mode":"smart-reasoning"}}`)))
	require.Equal(t, http.StatusForbidden, w.Code)

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		request := httptest.NewRequest(method, "/api/v1/system/admin/agents/builtin-skill-installer", bytes.NewBufferString(`{"config":{"agent_mode":"smart-reasoning"}}`))
		request.Header.Set("X-Test-API-Key", "true")
		response := httptest.NewRecorder()
		r.ServeHTTP(response, request)
		require.Equal(t, http.StatusForbidden, response.Code)
	}
}

func TestPlatformAgentHTTPRoundTripIsTenantless(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous, existed := types.BuiltinAgentRegistry[types.BuiltinSkillInstallerID]
	types.BuiltinAgentRegistry[types.BuiltinSkillInstallerID] = func(tenantID uint64) *types.CustomAgent {
		return &types.CustomAgent{ID: types.BuiltinSkillInstallerID, TenantID: tenantID, IsBuiltin: true, Name: "Skill Installer", Config: types.CustomAgentConfig{AgentMode: types.AgentModeSmartReasoning}}
	}
	restore := types.OverrideBuiltinAgentEntriesForTest(map[string]*types.BuiltinAgentEntry{
		types.BuiltinSkillInstallerID: {ID: types.BuiltinSkillInstallerID, IsBuiltin: true, I18n: map[string]types.BuiltinAgentI18n{"default": {Name: "Skill Installer"}}, Config: types.CustomAgentConfig{AgentMode: types.AgentModeSmartReasoning}},
	})
	t.Cleanup(func() {
		restore()
		if existed {
			types.BuiltinAgentRegistry[types.BuiltinSkillInstallerID] = previous
		} else {
			delete(types.BuiltinAgentRegistry, types.BuiltinSkillInstallerID)
		}
	})

	repo := &platformAgentHTTPRepo{}
	svc := service.NewPlatformAgentService(repo, platformAgentHTTPModels{})
	h := handler.NewPlatformAgentHandler(svc, &config.Config{})
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.SystemAdminContextKey, true)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	g := &rbacGuards{cfg: &config.Config{}, apiKeyAuthorizer: middleware.NewAPIKeyRouteAuthorizer()}
	RegisterPlatformAgentRoutes(r.Group("/api/v1"), h, g)

	body, err := json.Marshal(handler.UpdatePlatformAgentRequest{Config: types.CustomAgentConfig{AgentMode: types.AgentModeSmartReasoning, SystemPrompt: "saved globally"}})
	require.NoError(t, err)
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/system/admin/agents", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/system/admin/agents/builtin-skill-installer", nil),
		httptest.NewRequest(http.MethodPut, "/api/v1/system/admin/agents/builtin-skill-installer", bytes.NewReader(body)),
	} {
		request.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	require.NotNil(t, repo.row)
	require.Zero(t, repo.row.TenantID)
	require.Equal(t, types.BuiltinSkillInstallerID, repo.row.ID)
	require.Equal(t, "saved globally", repo.row.Config.SystemPrompt)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/system/admin/agents"},
		{http.MethodGet, "/api/v1/system/admin/agents/:id"},
		{http.MethodPut, "/api/v1/system/admin/agents/:id"},
	} {
		if _, ok := g.apiKeyAuthorizer.Lookup(route.method, route.path); ok {
			t.Fatalf("platform agent browser routes must remain default-denied to API keys: %s %s", route.method, route.path)
		}
	}
}
