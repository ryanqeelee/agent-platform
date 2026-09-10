package service

import (
	"context"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNativeAnalysisAgentUsesExistingTenantSandboxWithoutEmployeeScenario(t *testing.T) {
	restore := types.OverrideBuiltinAgentEntriesForTest(map[string]*types.BuiltinAgentEntry{
		"builtin-operating-analyst":  {ID: "builtin-operating-analyst", IsBuiltin: true, Config: types.CustomAgentConfig{SystemPrompt: "operating", AllowedTools: []string{"governed_data_schema", "governed_data_query"}, SkillsSelectionMode: "all"}},
		"builtin-data-analysis-base": {ID: "builtin-data-analysis-base", IsBuiltin: true, Config: types.CustomAgentConfig{SystemPrompt: "base", AllowedTools: []string{"data_schema", "data_analysis"}}},
	})
	t.Cleanup(restore)
	own := &types.TenantSandboxConfigEntity{ID: "ours", TenantID: 7, Name: "employee-assistant"}
	for _, id := range []string{"builtin-operating-analyst", "builtin-data-analysis-base"} {
		for _, tc := range []struct {
			name   string
			rows   []*types.TenantSandboxConfigEntity
			reject bool
		}{
			{"own", []*types.TenantSandboxConfigEntity{own}, false},
			{"missing", nil, false},
			{"duplicate", []*types.TenantSandboxConfigEntity{own, own}, true},
			{"foreign", []*types.TenantSandboxConfigEntity{{ID: "foreign", TenantID: 8, Name: "employee-assistant"}}, true},
		} {
			t.Run(id+tc.name, func(t *testing.T) {
				svc := &customAgentService{scenarioCapabilities: assistantScenarioResolverStub{settings: assistantScenarioSettings(false, false, true)}, sandboxConfigs: &employeeSandboxRepoStub{rows: tc.rows}}
				ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
				agent, err := svc.GetAgentByID(ctx, id)
				if tc.reject {
					require.Error(t, err)
					return
				}
				require.NoError(t, err)
				if tc.name == "missing" {
					require.Empty(t, agent.Config.SandboxConfigID)
					require.Equal(t, "none", agent.Config.SkillsSelectionMode)
				} else {
					require.Equal(t, "ours", agent.Config.SandboxConfigID)
				}
				require.Equal(t, id, agent.ID)
			})
		}
	}
}

func TestNativeOperatingToolsSwitchDoesNotOwnGovernedSQL(t *testing.T) {
	restore := types.OverrideBuiltinAgentEntriesForTest(map[string]*types.BuiltinAgentEntry{
		types.BuiltinOperatingAnalystID: {ID: types.BuiltinOperatingAnalystID, IsBuiltin: true, Config: types.CustomAgentConfig{AgentMode: types.AgentModeSmartReasoning, AllowedTools: []string{"governed_data_schema", "governed_data_query"}, SkillsSelectionMode: "all"}},
	})
	t.Cleanup(restore)
	for _, enabled := range []bool{false, true} {
		resolver := assistantScenarioResolverStub{settings: assistantScenarioSettings(false, false, enabled)}
		svc := &customAgentService{scenarioCapabilities: resolver, sandboxConfigs: &employeeSandboxRepoStub{rows: []*types.TenantSandboxConfigEntity{{ID: "ours", TenantID: 7, Name: "employee-assistant"}}}}
		agent, err := svc.nativeAnalysisAgent(context.Background(), types.BuiltinOperatingAnalystID, 7)
		require.NoError(t, err)
		require.Equal(t, enabled, agent.Config.SandboxConfigID != "")
		require.NoError(t, validateAssistantScenarioExecution(context.Background(), resolver, nil, 7, agent, &types.QARequest{}))
		if !enabled {
			ordinary := *agent
			ordinary.ID = "ordinary"
			require.Error(t, validateAssistantScenarioExecution(context.Background(), resolver, nil, 7, &ordinary, &types.QARequest{}))
		}
	}
}

func TestNativeAnalysisNetworkOffDoesNotRequireNetworkPermission(t *testing.T) {
	resolver := assistantScenarioResolverStub{settings: assistantScenarioSettings(false, false, false)}
	for _, id := range []string{types.BuiltinOperatingAnalystID, types.BuiltinDataAnalysisBaseID} {
		agent := &types.CustomAgent{ID: id, Config: types.CustomAgentConfig{AgentMode: types.AgentModeSmartReasoning, WebSearchEnabled: true, AllowedTools: []string{"web_search", "web_fetch", "data_schema"}}}
		require.NoError(t, validateAssistantScenarioExecution(context.Background(), resolver, nil, 7, agent, &types.QARequest{WebSearchEnabled: false}))
		require.Error(t, validateAssistantScenarioExecution(context.Background(), resolver, nil, 7, agent, &types.QARequest{WebSearchEnabled: true}))
	}
}
