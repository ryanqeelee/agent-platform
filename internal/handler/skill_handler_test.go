package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type fakeUsableSkillLister struct {
	tenantID uint64
	configID string
	skills   []*types.TenantSkillEntity
}

type employeeSkillManagerStub struct {
	tenant        uint64
	config, skill string
	update        service.SkillAdminUpdate
	called        bool
}

func (s *employeeSkillManagerStub) ListSkills(_ context.Context, tenant uint64, config string) ([]*types.TenantSkillEntity, error) {
	s.tenant, s.config, s.called = tenant, config, true
	return []*types.TenantSkillEntity{{ID: "skill-1", Name: "pdf", SandboxConfigID: "private-config"}}, nil
}
func (s *employeeSkillManagerStub) UpdateSkillAdmin(_ context.Context, tenant uint64, config, skill string, update service.SkillAdminUpdate) (*types.TenantSkillEntity, error) {
	s.tenant, s.config, s.skill, s.update, s.called = tenant, config, skill, update, true
	return &types.TenantSkillEntity{ID: skill}, nil
}

func TestEnterpriseSkillsUseCanonicalTenantConfigAndOnlyChangeVisibility(t *testing.T) {
	for _, tc := range []struct {
		name, body, config string
		tenant             uint64
		status             int
		called             bool
	}{
		{"enabled", `{"enabled":true,"sandbox_config_id":"foreign","env_values":{"TOKEN":"ignored"}}`, "ours", testSkillTenantID, 200, true},
		{"disabled", `{"enabled":false}`, "ours", testSkillTenantID, 200, true},
		{"missing-toggle", `{}`, "ours", testSkillTenantID, 400, false},
		{"invalid-toggle", `{"enabled":"true"}`, "ours", testSkillTenantID, 400, false},
		{"unconfigured", `{"enabled":true}`, "", testSkillTenantID, 409, false},
		{"foreign-tenant", `{"enabled":true}`, "foreign", testSkillTenantID + 1, 503, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := &employeeSkillManagerStub{}
			h := NewSkillHandler(nil, nil).WithEmployeeManagement(manager).WithEmployeeAgents(employeeSkillAgentStub{agent: &types.CustomAgent{TenantID: tc.tenant, Config: types.CustomAgentConfig{SandboxConfigID: tc.config}}})
			r := newChatSkillRouter(h)
			r.PATCH("/manage/:id", h.SetEnterpriseSkillEnabled)
			w := httptest.NewRecorder()
			req := httptest.NewRequest("PATCH", "/manage/skill-1?sandbox_config_id=foreign", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.called, manager.called)
			if manager.called {
				require.Equal(t, testSkillTenantID, manager.tenant)
				require.Equal(t, "ours", manager.config)
				require.Equal(t, "skill-1", manager.skill)
				require.Empty(t, manager.update.EnvValues)
				require.Equal(t, tc.name == "enabled", *manager.update.Enabled)
			}
		})
	}
}

func TestEnterpriseSkillListExposesOnlyManagementMetadata(t *testing.T) {
	manager := &employeeSkillManagerStub{}
	h := NewSkillHandler(nil, nil).WithEmployeeManagement(manager).WithEmployeeAgents(employeeSkillAgentStub{agent: &types.CustomAgent{TenantID: testSkillTenantID, Config: types.CustomAgentConfig{SandboxConfigID: "ours"}}})
	r := newChatSkillRouter(h)
	r.GET("/manage", h.ListEnterpriseSkills)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/manage?sandbox_config_id=foreign", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "ours", manager.config)
	require.NotContains(t, w.Body.String(), "private-config")
	var body struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	require.Len(t, body.Data[0], 5)
}

func (f *fakeUsableSkillLister) ListUsableSkills(
	_ context.Context, tenantID uint64, configID string,
) []*types.TenantSkillEntity {
	f.tenantID = tenantID
	f.configID = configID
	if configID == "" {
		return nil
	}
	return f.skills
}

func newChatSkillRouter(h *SkillHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), testSkillTenantID)
		c.Next()
	})
	r.GET("/system/admin/tenants/42/skills", h.ListSkills)
	return r
}

func TestListSkillsHidesThePickerWhenNoSandboxConfigIsSelected(t *testing.T) {
	lister := &fakeUsableSkillLister{
		skills: []*types.TenantSkillEntity{{Name: "ppt-generator", Description: "make ppt"}},
	}
	router := newChatSkillRouter(NewSkillHandler(lister, nil))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/system/admin/tenants/42/skills", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Success         bool `json:"success"`
		Data            []SkillInfoResponse
		SkillsAvailable bool `json:"skills_available"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Empty(t, body.Data, "preloaded and unscoped skills must not appear in @")
	require.False(t, body.SkillsAvailable)
	require.Empty(t, lister.configID)
}

func TestListSkillsReturnsUsableInstalledSkillsForTheSelectedConfig(t *testing.T) {
	lister := &fakeUsableSkillLister{
		skills: []*types.TenantSkillEntity{
			{Name: "ppt-generator", Description: "make ppt"},
		},
	}
	router := newChatSkillRouter(NewSkillHandler(lister, nil))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(
		http.MethodGet, "/system/admin/tenants/42/skills?sandbox_config_id=cfg-1", nil,
	))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, testSkillTenantID, lister.tenantID)
	require.Equal(t, "cfg-1", lister.configID)

	var body struct {
		Success         bool `json:"success"`
		Data            []SkillInfoResponse
		SkillsAvailable bool `json:"skills_available"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.True(t, body.SkillsAvailable)
	require.Equal(t, []SkillInfoResponse{
		{Name: "ppt-generator", Description: "make ppt"},
	}, body.Data)
}

type employeeSkillAgentStub struct {
	interfaces.CustomAgentService
	agent *types.CustomAgent
}

func (s employeeSkillAgentStub) GetAgentByID(_ context.Context, id string) (*types.CustomAgent, error) {
	if id != types.BuiltinEmployeeAssistantID {
		panic("unexpected agent")
	}
	return s.agent, nil
}
func TestEmployeeSkillPickerIgnoresClientSandboxSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tenant uint64
		config string
		status int
	}{
		{"enabled", testSkillTenantID, "ours", 200},
		{"disabled", testSkillTenantID, "", 200},
		{"foreign", testSkillTenantID + 1, "foreign", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lister := &fakeUsableSkillLister{skills: []*types.TenantSkillEntity{{Name: "文档分析", Description: "分析文档"}}}
			agent := &types.CustomAgent{TenantID: tc.tenant, Config: types.CustomAgentConfig{SandboxConfigID: tc.config, SkillsSelectionMode: "all"}}
			h := NewSkillHandler(lister, nil).WithEmployeeAgents(employeeSkillAgentStub{agent: agent})
			router := newChatSkillRouter(h)
			router.GET("/employee-assistant/skills", h.ListEmployeeSkills)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/employee-assistant/skills?sandbox_config_id=other-tenant", nil))
			require.Equal(t, tc.status, w.Code)
			if tc.name == "enabled" {
				require.Equal(t, "ours", lister.configID)
				require.Equal(t, testSkillTenantID, lister.tenantID)
			} else {
				require.Empty(t, lister.configID)
			}
			require.NotContains(t, w.Body.String(), "sandbox_config_id")
		})
	}
}
