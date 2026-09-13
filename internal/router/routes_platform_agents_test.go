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
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantRoleContextKey, types.TenantRoleOwner)
		if c.GetHeader("X-Test-System-Admin") == "true" {
			ctx = context.WithValue(ctx, types.SystemAdminContextKey, true)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	h := &handler.PlatformAgentHandler{}
	RegisterPlatformAgentRoutes(r.Group("/api/v1"), h, &rbacGuards{cfg: &config.Config{}})

	for _, path := range []string{
		"/api/v1/system/admin/agents",
		"/api/v1/system/admin/agents/builtin-quick-answer",
		"/api/v1/system/admin/agents/type-presets",
		"/api/v1/system/admin/agents/placeholders",
		"/api/v1/system/admin/agents/prompt-templates",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusForbidden, w.Code, path)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/v1/system/admin/agents/builtin-quick-answer", bytes.NewBufferString(`{"config":{"agent_mode":"quick-answer"}}`)))
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestPlatformAgentHTTPRoundTripIsTenantless(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous, existed := types.BuiltinAgentRegistry[types.BuiltinQuickAnswerID]
	types.BuiltinAgentRegistry[types.BuiltinQuickAnswerID] = func(tenantID uint64) *types.CustomAgent {
		return &types.CustomAgent{ID: types.BuiltinQuickAnswerID, TenantID: tenantID, IsBuiltin: true, Name: "Quick Answer", Config: types.CustomAgentConfig{AgentMode: types.AgentModeQuickAnswer}}
	}
	restore := types.OverrideBuiltinAgentEntriesForTest(map[string]*types.BuiltinAgentEntry{
		types.BuiltinQuickAnswerID: {ID: types.BuiltinQuickAnswerID, IsBuiltin: true, I18n: map[string]types.BuiltinAgentI18n{"default": {Name: "Quick Answer"}}, Config: types.CustomAgentConfig{AgentMode: types.AgentModeQuickAnswer}},
	})
	t.Cleanup(func() {
		restore()
		if existed {
			types.BuiltinAgentRegistry[types.BuiltinQuickAnswerID] = previous
		} else {
			delete(types.BuiltinAgentRegistry, types.BuiltinQuickAnswerID)
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

	body, err := json.Marshal(handler.UpdatePlatformAgentRequest{Config: types.CustomAgentConfig{AgentMode: types.AgentModeQuickAnswer, SystemPrompt: "saved globally", KBSelectionMode: "all"}})
	require.NoError(t, err)
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/system/admin/agents", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/system/admin/agents/builtin-quick-answer", nil),
		httptest.NewRequest(http.MethodPut, "/api/v1/system/admin/agents/builtin-quick-answer", bytes.NewReader(body)),
	} {
		request.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, request)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	require.NotNil(t, repo.row)
	require.Zero(t, repo.row.TenantID)
	require.Equal(t, "saved globally", repo.row.Config.SystemPrompt)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/system/admin/agents"},
		{http.MethodPut, "/api/v1/system/admin/agents/:id"},
	} {
		if _, ok := g.apiKeyAuthorizer.Lookup(route.method, route.path); ok {
			t.Fatalf("platform agent browser routes must remain default-denied to API keys: %s %s", route.method, route.path)
		}
	}
}
