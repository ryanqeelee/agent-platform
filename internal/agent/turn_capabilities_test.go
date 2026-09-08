package agent

import (
	"context"
	"fmt"
	"testing"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestEmployeeMixedTaskKeepsOneLoopAndPriorEvidence(t *testing.T) {
	names := []string{agenttools.ToolKnowledgeSearch, agenttools.ToolReadSkill, agenttools.ToolExecuteSkillScript}
	registry := agenttools.NewToolRegistry()
	model := &mockChat{}
	for i, name := range names {
		registry.RegisterTool(newCountingTool(name))
		model.responses = append(model.responses, mockResponse{chunks: []types.StreamResponse{{
			ResponseType: types.ResponseTypeAnswer, Done: true, FinishReason: "tool_calls",
			ToolCalls: []types.LLMToolCall{{ID: fmt.Sprintf("call-%d", i), Type: "function", Function: types.FunctionCall{Name: name, Arguments: `{}`}}},
		}}})
	}
	model.responses = append(model.responses, mockResponse{chunks: []types.StreamResponse{{
		ResponseType: types.ResponseTypeAnswer, Content: "培训表已生成。", Done: true, FinishReason: "stop",
	}}})
	engine := NewAgentEngine(&types.AgentConfig{EmployeeAssistant: true, MaxIterations: 12, KnowledgeBases: []string{"kb"},
		SessionAttachments: types.MessageAttachments{{ID: "previous-table", FileName: "上轮附件.csv", FileType: "csv"}},
	}, model, registry, nil, nil, nil, "session", "员工助理")
	require.NotNil(t, engine)
	state, err := engine.Execute(context.Background(), "session", "message", "按制度核对上轮附件，再生成培训表", nil)
	require.NoError(t, err)
	require.True(t, state.IsComplete)
	require.Equal(t, 4, model.callCount, "no separate classifier or nested answering call")
	for _, name := range names {
		tool, err := registry.GetTool(name)
		require.NoError(t, err)
		require.Equal(t, 1, tool.(*countingTool).calls)
	}
	require.Contains(t, model.calls[0][1].Content, "上轮附件.csv")
	toolResults := 0
	for _, message := range model.calls[3] {
		if message.Role == "tool" {
			toolResults++
		}
	}
	require.Equal(t, 3, toolResults, "retain evidence across the whole mixed task")
}

func TestEmployeeTurnCapabilitiesFollowEffectiveTools(t *testing.T) {
	for _, tc := range []struct {
		name       string
		kbIDs      []string
		registered []string
		want       string
	}{
		{"no_scope", nil, nil, `<enterprise_knowledge scope="none_in_scope" tools_available="false"/>`},
		{"scope_without_tools", []string{"kb"}, nil, `<enterprise_knowledge scope="configured" tools_available="false"/>`},
		{"retrieval", []string{"kb"}, []string{agenttools.ToolKnowledgeSearch}, `<enterprise_knowledge scope="configured" tools_available="true"/>`},
		{"wiki_only", []string{"kb"}, []string{agenttools.ToolWikiSearch}, `<enterprise_knowledge scope="configured" tools_available="true"/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := agenttools.NewToolRegistry()
			for _, name := range tc.registered {
				registry.RegisterTool(newCountingTool(name))
			}
			cfg := &types.AgentConfig{EmployeeAssistant: true, KnowledgeBases: tc.kbIDs,
				AllowedTools: []string{agenttools.ToolKnowledgeSearch, agenttools.ToolWebSearch}, WebSearchEnabled: true}
			engine := NewAgentEngine(cfg, &mockChat{}, registry, nil, nil, nil, "session", "")
			require.NotNil(t, engine)
			content := engine.RenderUserTurnContent("session", "question")
			require.Contains(t, content, tc.want)
			require.Contains(t, content, `<public_web search_available="false" fetch_available="false"/>`, "configuration is not tool availability")
			registry.RegisterTool(newCountingTool(agenttools.ToolWebFetch))
			registry.RegisterTool(newCountingTool(agenttools.ToolReadSkill))
			content = engine.RenderUserTurnContent("session", "question")
			require.Contains(t, content, `<public_web search_available="false" fetch_available="true"/>`)
			require.Contains(t, content, `<skills read_available="true" execute_available="false"/>`)
			require.Contains(t, content, "distinguish completed work from unresolved requirements")
			cfg.EmployeeAssistant = false
			require.NotContains(t, engine.RenderUserTurnContent("session", "question"), "<turn_capabilities>")
		})
	}
}

// A scripted model verifies execution shape, not real model decision quality.
func TestEmployeeDirectAnswerUsesOneModelCallAndNoTools(t *testing.T) {
	model := &mockChat{responses: []mockResponse{{chunks: []types.StreamResponse{{
		ResponseType: types.ResponseTypeAnswer, Content: "以下是通用入职清单，不代表本企业制度。", Done: true, FinishReason: "stop",
	}}}}}
	registry := agenttools.NewToolRegistry()
	read := newCountingTool(agenttools.ToolReadSkill)
	shell := newCountingTool(agenttools.ToolShellExec)
	registry.RegisterTool(read)
	registry.RegisterTool(shell)
	engine := NewAgentEngine(&types.AgentConfig{EmployeeAssistant: true, MaxIterations: 12}, model, registry, nil, nil, nil, "session", "员工助理")
	require.NotNil(t, engine)
	state, err := engine.Execute(context.Background(), "session", "message", "新员工上岗前需要了解哪些服务规范？", nil)
	require.NoError(t, err)
	require.True(t, state.IsComplete)
	require.Contains(t, state.FinalAnswer, "通用入职清单")
	require.Equal(t, 1, model.callCount)
	require.Zero(t, read.calls)
	require.Zero(t, shell.calls)
	require.Contains(t, model.calls[0][1].Content, `scope="none_in_scope"`)
}
