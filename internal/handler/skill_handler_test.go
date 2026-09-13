package handler

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type fakeUsableSkillLister struct {
	tenantID uint64
	configID string
	skills   []*types.TenantSkillEntity
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

func newEmployeeSkillRouter(h *SkillHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(func(c *gin.Context) {
		c.Set(types.TenantIDContextKey.String(), testSkillTenantID)
		c.Next()
	})
	r.GET("/employee-assistant/skills", h.ListEmployeeSkills)
	return r
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
			router := newEmployeeSkillRouter(h)
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
