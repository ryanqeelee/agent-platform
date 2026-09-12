package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type skillInstallerAgentService struct {
	interfaces.CustomAgentService
	getID          string
	getTenantID    uint64
	updated        *types.CustomAgent
	updateTenantID uint64
}

func (s *skillInstallerAgentService) GetAgentByID(ctx context.Context, id string) (*types.CustomAgent, error) {
	s.getID = id
	s.getTenantID, _ = types.TenantIDFromContext(ctx)
	return &types.CustomAgent{
		ID:        id,
		Name:      "Skill installer",
		TenantID:  s.getTenantID,
		IsBuiltin: true,
		Config:    types.CustomAgentConfig{ModelID: "model-before"},
	}, nil
}

func (s *skillInstallerAgentService) UpdateAgent(
	ctx context.Context,
	agent *types.CustomAgent,
) (*types.CustomAgent, error) {
	s.updated = agent
	s.updateTenantID, _ = types.TenantIDFromContext(ctx)
	agent.TenantID = s.updateTenantID
	agent.IsBuiltin = true
	return agent, nil
}

func skillInstallerHandlerTestRouter(h *CustomAgentHandler) *gin.Engine {
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.TenantIDContextKey, uint64(42))
		ctx = context.WithValue(ctx, types.SystemAdminContextKey, true)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/system/admin/tenants/42/skills/installer-agent", h.GetSkillInstallerAgent)
	router.PUT("/system/admin/tenants/42/skills/installer-agent", h.UpdateSkillInstallerAgent)
	router.GET("/agents/:id", h.GetAgent)
	return router
}

func TestSkillInstallerAgentHandlersUseFixedBuiltinIDAndTenantContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &skillInstallerAgentService{}
	router := skillInstallerHandlerTestRouter(&CustomAgentHandler{service: service})

	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, httptest.NewRequest(
		http.MethodGet,
		"/system/admin/tenants/42/skills/installer-agent",
		nil,
	))
	require.Equal(t, http.StatusOK, getResponse.Code, getResponse.Body.String())
	require.Equal(t, types.BuiltinSkillInstallerID, service.getID)
	require.Equal(t, uint64(42), service.getTenantID)
	require.Contains(t, getResponse.Body.String(), `"model_id":"model-before"`)

	updateResponse := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPut,
		"/system/admin/tenants/42/skills/installer-agent",
		strings.NewReader(`{"name":"ignored for builtin","config":{"model_id":"model-after"}}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(updateResponse, request)
	require.Equal(t, http.StatusOK, updateResponse.Code, updateResponse.Body.String())
	require.NotNil(t, service.updated)
	require.Equal(t, types.BuiltinSkillInstallerID, service.updated.ID)
	require.Equal(t, uint64(42), service.updateTenantID)
	require.Equal(t, "model-after", service.updated.Config.ModelID)
}

func TestGenericAgentHandlerStillUsesRouteID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &skillInstallerAgentService{}
	router := skillInstallerHandlerTestRouter(&CustomAgentHandler{service: service})

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/agents/enterprise-agent", nil))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "enterprise-agent", service.getID)
}
