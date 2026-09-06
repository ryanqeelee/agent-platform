package agent

import (
	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/modelcontext"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNormalizeAndDecodeNullableWebFetchItems(t *testing.T) {
	registry := tools.NewToolRegistry()
	registry.RegisterTool(tools.NewWebFetchTool(nil))
	e := &AgentEngine{toolRegistry: registry, modelContext: modelcontext.NewRegistry(true)}
	e.modelContext.RegisterWeb("https://example.com/page", "Example")
	raw := `{"items":"[{\"url\":\"w1\",\"prompt\":\"read\"}]"}`
	calls := []types.LLMToolCall{{Function: types.FunctionCall{Name: tools.ToolWebFetch, Arguments: raw}}}
	e.normalizeAndDecodeToolCalls(calls)
	require.Equal(t, raw, calls[0].ModelArguments)
	require.JSONEq(t, `{"items":[{"url":"https://example.com/page","prompt":"read"}]}`, calls[0].Function.Arguments)
	require.Empty(t, calls[0].UnresolvedHandles)
	resolved := calls[0].Function.Arguments
	e.normalizeAndDecodeToolCalls(calls)
	require.Equal(t, resolved, calls[0].Function.Arguments)
	require.Equal(t, raw, calls[0].ModelArguments)
	calls = []types.LLMToolCall{{Function: types.FunctionCall{Name: tools.ToolWebFetch, Arguments: `{"items":"[{\"url\":\"w99\",\"prompt\":\"read\"}]"}`}}}
	e.normalizeAndDecodeToolCalls(calls)
	require.Contains(t, calls[0].UnresolvedHandles, "w99")
	calls = []types.LLMToolCall{{Function: types.FunctionCall{Name: tools.ToolWebFetch, Arguments: `{"items":"[`}}}
	e.normalizeAndDecodeToolCalls(calls)
	require.Equal(t, `{"items":"[`, calls[0].Function.Arguments)
}
