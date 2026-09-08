package service

import (
	"context"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEmployeeQuickProfilePreservesContractForIntentOverride(t *testing.T) {
	for _, id := range []string{types.BuiltinEmployeeAssistantID, types.BuiltinQuickAnswerID} {
		agent := &types.CustomAgent{ID: id, Config: types.CustomAgentConfig{AgentMode: types.AgentModeQuickAnswer, SystemPrompt: "employee evidence rules"}}
		cm := &types.ChatManage{}
		svc := &sessionService{}
		svc.applyAgentOverridesToChatManage(context.Background(), agent, cm)
		require.Equal(t, "employee evidence rules", cm.SummaryConfig.Prompt)
		if id == types.BuiltinEmployeeAssistantID {
			require.Equal(t, "employee evidence rules", cm.SystemPromptOverride)
			require.Contains(t, cm.RewritePromptSystem, "needs_user_input")
		} else {
			require.Empty(t, cm.SystemPromptOverride)
		}
	}
}
