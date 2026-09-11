package session

import (
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEmployeeModeProfiles(t *testing.T) {
	original := &types.CustomAgent{ID: types.BuiltinEmployeeAssistantID, TenantID: 10003, Config: types.CustomAgentConfig{
		AgentMode: types.AgentModeSmartReasoning, ModelID: "tenant-model", KBSelectionMode: "all",
		AllowedTools: []string{"knowledge_search", "data_analysis"}, DataAnalysisEnabled: true,
		MultiTurnEnabled: true, HistoryTurns: 10, SystemPrompt: "employee contract",
	}}
	for _, mode := range []string{"", "quick", "deep"} {
		t.Run("employee_"+mode, func(t *testing.T) {
			got, selected, err := resolveEmployeeMode(original, mode)
			require.NoError(t, err)
			require.NotSame(t, original, got)
			require.Equal(t, original.ID, got.ID)
			require.Equal(t, original.TenantID, got.TenantID)
			require.Equal(t, original.Config.ModelID, got.Config.ModelID)
			require.Equal(t, original.Config.KBSelectionMode, got.Config.KBSelectionMode)
			require.Equal(t, original.Config.MemoryEnabled, got.Config.MemoryEnabled)
			require.Equal(t, original.Config.HistoryTurns, got.Config.HistoryTurns)
			require.True(t, got.Config.MultiTurnEnabled)
			if mode == "deep" {
				require.Equal(t, "deep", selected)
				require.Equal(t, original.Config, got.Config)
			} else {
				require.Equal(t, "quick", selected)
				require.False(t, got.IsAgentMode())
				require.False(t, got.Config.DataAnalysisEnabled)
				require.Empty(t, got.Config.AllowedTools)
				require.Contains(t, got.Config.SystemPrompt, "employee contract")
				require.Contains(t, got.Config.SystemPrompt, "实际执行结果")
				require.False(t, got.Config.WebSearchEnabled)
				require.Contains(t, got.Config.SystemPrompt, "不会扩大企业知识权限")
			}
		})
	}
	require.True(t, original.IsAgentMode())
	require.True(t, original.Config.DataAnalysisEnabled)
	require.Len(t, original.Config.AllowedTools, 2)
}

func TestEmployeeModeRejectsInvalidAndPreservesOtherAgents(t *testing.T) {
	for _, id := range []string{types.BuiltinQuickAnswerID, types.BuiltinSmartReasoningID, "custom"} {
		agent := &types.CustomAgent{ID: id}
		got, mode, err := resolveEmployeeMode(agent, "")
		require.NoError(t, err)
		require.Same(t, agent, got)
		require.Empty(t, mode)
		for _, requested := range []string{"quick", "deep", "invalid"} {
			_, _, err = resolveEmployeeMode(agent, requested)
			require.Error(t, err)
		}
	}
	_, _, err := resolveEmployeeMode(nil, "quick")
	require.Error(t, err)
	_, _, err = resolveEmployeeMode(&types.CustomAgent{ID: types.BuiltinEmployeeAssistantID}, "invalid")
	require.Error(t, err)
}
