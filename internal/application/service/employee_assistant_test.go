package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type employeeProviderStub struct {
	interfaces.WebSearchProviderRepository
	provider *types.WebSearchProviderEntity
	err      error
}

type employeeModelsStub struct{ interfaces.ModelService }

func (employeeModelsStub) GetModelByID(_ context.Context, id string) (*types.Model, error) {
	if id == "employee-model" {
		return &types.Model{ID: id, Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive}, nil
	}
	return nil, nil
}

func (employeeModelsStub) ListModels(context.Context) ([]*types.Model, error) {
	return []*types.Model{
		{ID: "larger-alternative", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive},
		{ID: "platform-flash", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive, IsDefault: true},
	}, nil
}
func TestEmployeeAssistantUsesConfiguredModelAndFallsBackWhenUnavailable(t *testing.T) {
	svc := &sessionService{modelService: employeeModelsStub{}}
	for _, tc := range []struct {
		name       string
		configured string
		want       string
	}{
		{name: "configured binding", configured: "employee-model", want: "employee-model"},
		{name: "unavailable binding", configured: "missing-model", want: "platform-flash"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, mode := range []string{types.AgentModeQuickAnswer, types.AgentModeSmartReasoning} {
				t.Run(mode, func(t *testing.T) {
					id, err := svc.resolveChatModelID(context.Background(), &types.QARequest{
						CustomAgent: &types.CustomAgent{
							ID: types.BuiltinEmployeeAssistantID,
							Config: types.CustomAgentConfig{
								AgentMode: mode,
								ModelID:   tc.configured,
							},
						},
						SummaryModelID: "client-override",
						Session:        &types.Session{},
					}, nil, nil, nil)
					require.NoError(t, err)
					require.Equal(t, tc.want, id)
				})
			}
		})
	}
}
func (s *employeeProviderStub) GetDefault(_ context.Context) (*types.WebSearchProviderEntity, error) {
	return s.provider, s.err
}

func TestEmployeeAssistantReadinessAndCanonicalConfig(t *testing.T) {
	restore := types.OverrideBuiltinAgentEntriesForTest(map[string]*types.BuiltinAgentEntry{
		types.BuiltinEmployeeAssistantID: {ID: types.BuiltinEmployeeAssistantID, IsBuiltin: true,
			Config: types.CustomAgentConfig{AgentMode: types.AgentModeSmartReasoning, ImageUploadEnabled: true,
				AllowedTools: []string{tools.ToolKnowledgeSearch, tools.ToolWikiSearch, tools.ToolDataAnalysis}, WebSearchEnabled: true}},
	})
	t.Cleanup(restore)
	for _, tc := range []struct {
		name     string
		enabled  bool
		provider *types.WebSearchProviderEntity
		err      error
		ready    bool
	}{
		{"enabled", true, &types.WebSearchProviderEntity{ID: "local", Provider: types.WebSearchProviderTypeKeenable}, nil, true},
		{"no_default", true, nil, nil, false},
		{"disabled", false, nil, nil, false},
		{"invalid_credentials", true, &types.WebSearchProviderEntity{ID: "local", Provider: types.WebSearchProviderTypeGoogle}, nil, false},
		{"storage_error", true, nil, errors.New("storage unavailable"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &employeeProviderStub{provider: tc.provider, err: tc.err}
			svc := &customAgentService{repo: &platformAgentRepoStub{rows: map[uint64]map[string]*types.CustomAgent{}}, scenarioCapabilities: assistantScenarioResolverStub{settings: assistantScenarioSettings(tc.enabled, false, false)}, webSearchProviders: provider}
			ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
			agent, err := svc.GetAgentByID(ctx, types.BuiltinEmployeeAssistantID)
			if tc.err != nil {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, uint64(7), agent.TenantID)
			require.Equal(t, tc.ready, *agent.WebSearchReady)
			require.True(t, agent.Config.ImageUploadEnabled)
			view := AgentView(ctx, agent)
			require.Equal(t, tc.ready, *view.WebSearchReady)
			require.Empty(t, view.Config.WebSearchProviderID)
		})
	}
}

func TestEmployeeReadToolsDoNotGrantExtensions(t *testing.T) {
	base := types.CustomAgentConfig{AgentMode: types.AgentModeSmartReasoning, AllowedTools: []string{tools.ToolKnowledgeSearch, tools.ToolWikiReadPage, tools.ToolDataSchema, tools.ToolDataAnalysis}}
	resolver := assistantScenarioResolverStub{settings: assistantScenarioSettings(false, false, false)}
	require.NoError(t, validateAssistantScenarioCapabilities(context.Background(), resolver, 7, base, false))
	for _, tool := range []string{"shell_exec", "wiki_write_page", "unregistered_tool", tools.ToolWebSearch, tools.ToolWebFetch} {
		cfg := base
		cfg.AllowedTools = append(append([]string{}, base.AllowedTools...), tool)
		require.ErrorIs(t, validateAssistantScenarioCapabilities(context.Background(), resolver, 7, cfg, false), ErrAssistantScenarioCapabilityDenied, tool)
	}
	for _, cfg := range []types.CustomAgentConfig{
		{SandboxConfigID: "previous-sandbox"},
		{SkillsSelectionMode: "all"},
		{SkillsSelectionMode: "selected", SelectedSkills: []string{"script"}},
		{MCPSelectionMode: "selected", MCPServices: []string{"external"}},
	} {
		require.ErrorIs(t, validateAssistantScenarioCapabilities(context.Background(), resolver, 7, cfg, false), ErrAssistantScenarioCapabilityDenied)
	}
}

type employeeSandboxDefaultStub struct {
	id    string
	err   error
	calls int
}

func (r *employeeSandboxDefaultStub) DefaultSandboxConfigID(context.Context) (string, error) {
	r.calls++
	return r.id, r.err
}
func TestEmployeeAssistantSandboxSelection(t *testing.T) {
	restore := types.OverrideBuiltinAgentEntriesForTest(map[string]*types.BuiltinAgentEntry{
		types.BuiltinEmployeeAssistantID: {ID: types.BuiltinEmployeeAssistantID, IsBuiltin: true},
	})
	t.Cleanup(restore)
	for _, tc := range []struct {
		name       string
		enabled    bool
		defaultID  string
		defaultErr error
		want       string
		wantErr    bool
	}{
		{name: "disabled", defaultID: "ours"},
		{name: "unconfigured", enabled: true},
		{name: "configured", enabled: true, defaultID: "ours", want: "ours"},
		{name: "read_failure", enabled: true, defaultErr: errors.New("unavailable"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defaults := &employeeSandboxDefaultStub{id: tc.defaultID, err: tc.defaultErr}
			svc := &customAgentService{repo: &platformAgentRepoStub{rows: map[uint64]map[string]*types.CustomAgent{}}, scenarioCapabilities: assistantScenarioResolverStub{settings: assistantScenarioSettings(false, false, tc.enabled)}, sandboxDefault: defaults}
			agent, err := svc.employeeAssistant(context.Background(), 7)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, agent.Config.SandboxConfigID)
			if tc.want != "" {
				require.Equal(t, "all", agent.Config.SkillsSelectionMode)
			} else {
				require.NotEqual(t, "all", agent.Config.SkillsSelectionMode)
			}
			if tc.enabled {
				require.Equal(t, 1, defaults.calls)
			} else {
				require.Zero(t, defaults.calls)
			}
		})
	}
}
func TestEmployeeSandboxDisabled(t *testing.T) {
	for _, tc := range []struct {
		config   types.AgentConfig
		disabled bool
	}{
		{types.AgentConfig{EmployeeAssistant: true}, true},
		{types.AgentConfig{EmployeeAssistant: true, SandboxConfigID: "stale"}, true},
		{types.AgentConfig{EmployeeAssistant: true, SkillsEnabled: true}, true},
		{types.AgentConfig{EmployeeAssistant: true, SkillsEnabled: true, SandboxConfigID: "selected"}, false},
		{types.AgentConfig{}, false},
	} {
		require.Equal(t, tc.disabled, employeeSandboxDisabled(&tc.config))
	}
}
