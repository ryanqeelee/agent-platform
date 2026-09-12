package agent

import (
	"context"
	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGovernedFailureReachesModelAndSafeStorage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"detail":"Catalog version changed; discover the current Catalog"}`))
	}))
	defer server.Close()
	client, err := agenttools.NewGovernedDataClient(types.GovernedEdgeConnection{BaseURL: server.URL, Token: "secret", EnterpriseID: "tenant", EdgeNodeID: "edge", SourceID: "retail"}, func(context.Context) error { return nil })
	require.NoError(t, err)
	engine := newTestEngine(t, &mockChat{})
	engine.toolRegistry = agenttools.NewToolRegistry()
	engine.toolRegistry.RegisterTool(agenttools.NewGovernedDataTools(client, nil, "session")[0])
	call := engine.runToolCall(context.Background(), types.LLMToolCall{ID: "call-1", Function: types.FunctionCall{Name: agenttools.ToolGovernedDataSchema, Arguments: `{}`}}, 0, 0, 1, "session", "message")
	require.False(t, call.Result.Success)
	require.Equal(t, "catalog_changed", call.Result.Data["governed_error_code"])
	messages := engine.appendToolResults(nil, types.AgentStep{ToolCalls: []types.ToolCall{call}})
	require.Contains(t, messages[len(messages)-1].Content, "without a table")
	stored := agenttools.SanitizeAgentStepsForStorage([]types.AgentStep{{ToolCalls: []types.ToolCall{call}}})
	require.Contains(t, stored[0].ToolCalls[0].Result.Error, "数据目录已更新")
	require.NotContains(t, stored[0].ToolCalls[0].Result.Error, "HTTP")
}
