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
	tenant   uint64
}

type employeeModelsStub struct{ interfaces.ModelService }

func (employeeModelsStub) ListModels(context.Context) ([]*types.Model, error) {
	return []*types.Model{
		{ID: "larger-alternative", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive},
		{ID: "platform-flash", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive, IsDefault: true},
	}, nil
}
func TestEmployeeAssistantUsesPlatformDefaultAcrossTopics(t *testing.T) {
	svc := &sessionService{modelService: employeeModelsStub{}}
	for _, targets := range [][]string{nil, {"different-kb"}} {
		id, err := svc.resolveChatModelID(context.Background(), &types.QARequest{
			CustomAgent:    &types.CustomAgent{ID: types.BuiltinEmployeeAssistantID, Config: types.CustomAgentConfig{ModelID: "stale-model"}},
			SummaryModelID: "client-override", Session: &types.Session{},
		}, targets, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "platform-flash", id)
	}
}
func (s *employeeProviderStub) GetDefault(_ context.Context, tenant uint64) (*types.WebSearchProviderEntity, error) {
	s.tenant = tenant
	return s.provider, s.err
}

func TestEmployeeAssistantReadinessAndCanonicalConfig(t *testing.T) {
	restore := types.OverrideBuiltinAgentEntriesForTest(map[string]*types.BuiltinAgentEntry{
		types.BuiltinEmployeeAssistantID: {ID: types.BuiltinEmployeeAssistantID, IsBuiltin: true,
			Config: types.CustomAgentConfig{AgentMode: types.AgentModeSmartReasoning, ImageUploadEnabled: true,
				AllowedTools: []string{tools.ToolKnowledgeSearch, tools.ToolWikiSearch, tools.ToolDataAnalysis}}},
	})
	t.Cleanup(restore)
	for _, tc := range []struct {
		name     string
		enabled  bool
		provider *types.WebSearchProviderEntity
		err      error
		ready    bool
	}{
		{"enabled", true, &types.WebSearchProviderEntity{ID: "local", TenantID: 7, Provider: types.WebSearchProviderTypeKeenable}, nil, true},
		{"no_default", true, nil, nil, false},
		{"other_tenant", true, &types.WebSearchProviderEntity{ID: "foreign", TenantID: 8, Provider: types.WebSearchProviderTypeKeenable}, nil, false},
		{"disabled", false, nil, nil, false},
		{"invalid_credentials", true, &types.WebSearchProviderEntity{ID: "local", TenantID: 7, Provider: types.WebSearchProviderTypeGoogle}, nil, false},
		{"storage_error", true, nil, errors.New("storage unavailable"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := &employeeProviderStub{provider: tc.provider, err: tc.err}
			svc := &customAgentService{scenarioCapabilities: assistantScenarioResolverStub{settings: assistantScenarioSettings(tc.enabled, false, false)}, webSearchProviders: provider}
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
			if tc.enabled {
				require.Equal(t, uint64(7), provider.tenant)
			} else {
				require.Zero(t, provider.tenant)
			}
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
