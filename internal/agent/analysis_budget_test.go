package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// Finite governed turns get one notice while tools still work. Other agents
// and unlimited turns keep their behaviour; no extra iteration is granted.
func TestAnalysisBudgetNoticePreservesToolsAndLimit(t *testing.T) {
	for _, name := range []string{agenttools.ToolGovernedDataQuery, "ordinary_tool"} {
		t.Run(name, func(t *testing.T) {
			model := &mockChat{}
			for i := 0; i < 8; i++ {
				model.responses = append(model.responses, mockResponse{chunks: []types.StreamResponse{{ToolCalls: []types.LLMToolCall{{ID: fmt.Sprintf("q%d", i), Function: types.FunctionCall{Name: name, Arguments: `{}`}}}, Done: true}}})
			}
			model.responses = append(model.responses, mockResponse{chunks: []types.StreamResponse{{ResponseType: types.ResponseTypeAnswer, Content: "supported answer", Done: true}}})
			engine := newTestEngine(t, model, withMaxIterations(8))
			registry := agenttools.NewToolRegistry()
			tool := newCountingTool(name)
			registry.RegisterTool(tool)
			engine.toolRegistry = registry
			available := []chat.Tool{{Type: "function", Function: chat.FunctionDef{Name: name}}}
			result, err := engine.executeLoop(context.Background(), &types.AgentState{}, "question", emptyMessages(), available, "sess", "msg")
			require.NoError(t, err)
			require.Equal(t, "supported answer", result.FinalAnswer)
			require.Equal(t, 8, tool.calls)
			require.Len(t, model.calls, 9)
			for i, call := range model.calls {
				notices := 0
				for _, msg := range call {
					if strings.HasPrefix(msg.Content, "Analysis execution budget:") {
						notices++
						require.Contains(t, msg.Content, "5 model iterations remain")
					}
				}
				expected := 0
				if name == agenttools.ToolGovernedDataQuery && i >= 3 {
					expected = 1
				}
				require.Equal(t, expected, notices, "call %d", i)
				if i < 8 {
					require.NotEqual(t, "none", model.opts[i].ToolChoice)
					require.Len(t, model.opts[i].Tools, 1)
				}
			}
			require.Equal(t, "none", model.opts[8].ToolChoice)
		})
	}
}

func TestAnalysisBudgetNoticeSkipsUnlimitedTurns(t *testing.T) {
	model := &mockChat{responses: []mockResponse{{chunks: []types.StreamResponse{{ResponseType: types.ResponseTypeAnswer, Content: "supported answer", Done: true}}}}}
	engine := newTestEngine(t, model, withMaxIterations(types.UnlimitedMaxIterations))
	available := []chat.Tool{{Type: "function", Function: chat.FunctionDef{Name: agenttools.ToolGovernedDataQuery}}}
	result, err := engine.executeLoop(context.Background(), &types.AgentState{}, "question", emptyMessages(), available, "sess", "msg")
	require.NoError(t, err)
	require.Equal(t, "supported answer", result.FinalAnswer)
	require.Len(t, model.calls, 1)
	for _, msg := range model.calls[0] {
		require.False(t, strings.HasPrefix(msg.Content, "Analysis execution budget:"))
	}
}
