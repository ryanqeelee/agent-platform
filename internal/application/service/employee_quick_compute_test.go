package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

type quickSandboxProbe struct {
	capableManager
	tenant              uint64
	session             string
	timeout             time.Duration
	begins, ends, calls int
	exitCode            int
}

func (m *quickSandboxProbe) BeginSessionTurn(context.Context, string) error { m.begins++; return nil }
func (m *quickSandboxProbe) EndSessionTurn(ctx context.Context, _ string) error {
	m.ends++
	return ctx.Err()
}
func (m *quickSandboxProbe) ExecShellCommand(ctx context.Context, sid, command, dir string, timeout time.Duration, env map[string]string) (*sandbox.ExecuteResult, error) {
	m.calls++
	m.tenant, _ = types.TenantIDFromContext(ctx)
	m.session = sid
	m.timeout = timeout
	return &sandbox.ExecuteResult{Stdout: "2", ExitCode: m.exitCode}, nil
}

type quickResolverProbe struct {
	mgr    sandbox.Manager
	calls  int
	tenant uint64
	config string
}

func (r *quickResolverProbe) Resolve(_ context.Context, tenant uint64, config string) (sandbox.Manager, error) {
	r.calls++
	r.tenant = tenant
	r.config = config
	return r.mgr, nil
}

func TestEmployeeQuickSandboxIsLazyBoundedAndSessionBound(t *testing.T) {
	mgr := &quickSandboxProbe{capableManager: capableManager{typ: sandbox.SandboxTypeE2B}}
	mgr.shell = mgr
	resolver := &quickResolverProbe{mgr: mgr}
	svc := &agentService{sandboxResolver: resolver}
	tool := svc.quickCompletionShellTool(7, "session-a", "named-config").(*quickCompletionShell)
	require.Zero(t, resolver.calls)
	require.Zero(t, mgr.begins)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(99))
	_, err := tool.executor.ExecShellCommand(ctx, "foreign-session", "echo 2", "", time.Hour, nil)
	require.Error(t, err)
	require.Zero(t, resolver.calls)
	for range 2 {
		_, err = tool.executor.ExecShellCommand(ctx, "session-a", "echo 2", "", time.Hour, nil)
		require.NoError(t, err)
	}
	require.Equal(t, 1, resolver.calls)
	require.EqualValues(t, 7, resolver.tenant)
	require.Equal(t, "named-config", resolver.config)
	require.EqualValues(t, 7, mgr.tenant)
	require.Equal(t, "session-a", mgr.session)
	require.Equal(t, employeeQuickShellTimeout, mgr.timeout)
	require.Equal(t, 1, mgr.begins)
	mgr.exitCode = 1
	callCtx := tools.WithToolExecContext(ctx, &tools.ToolExecContext{SessionID: "session-a"})
	result, err := tool.Execute(callCtx, json.RawMessage(`{"command":"python3 -c 'print(2)'"}`))
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Contains(t, result.Output, "exit_code=1")
	require.NotContains(t, result.Output, "execute_skill_script")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	tool.Cleanup(cancelled)
	require.Equal(t, 1, mgr.ends)
	require.Nil(t, svc.quickCompletionShellTool(7, "session-a", ""))
}

func TestEmployeeQuickCompletionUsesOnlyCanonicalQuickConfig(t *testing.T) {
	svc := &sessionService{agentService: &agentService{}}
	for _, tc := range []struct {
		id, mode, config string
		enabled          bool
	}{
		{types.BuiltinEmployeeAssistantID, types.AgentModeQuickAnswer, "named", true},
		{types.BuiltinEmployeeAssistantID, types.AgentModeSmartReasoning, "named", false},
		{types.BuiltinEmployeeAssistantID, types.AgentModeQuickAnswer, "", false},
		{"custom", types.AgentModeQuickAnswer, "named", false},
	} {
		t.Run(tc.id+tc.mode+tc.config, func(t *testing.T) {
			req := &types.QARequest{Session: &types.Session{ID: "s", TenantID: 7}, CustomAgent: &types.CustomAgent{ID: tc.id, Config: types.CustomAgentConfig{AgentMode: tc.mode, SandboxConfigID: tc.config}}}
			cm := &types.ChatManage{}
			svc.configureEmployeeQuickCompletion(req, cm)
			if tc.enabled {
				require.Len(t, cm.CompletionTools, 1)
				require.Equal(t, tools.ToolShellExec, cm.CompletionTools[0].Name())
				require.Equal(t, 4, cm.CompletionConfig.MaxIterations)
			} else {
				require.Empty(t, cm.CompletionTools)
				require.Nil(t, cm.CompletionConfig)
			}
		})
	}
}

type quickFallbackChat struct {
	captureChatModel
	calls int
	first []chat.Message
}

func (m *quickFallbackChat) ChatStream(_ context.Context, messages []chat.Message, options *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	m.calls++
	ch := make(chan types.StreamResponse, 1)
	if m.calls == 1 {
		m.first = messages
		ch <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Done: true, FinishReason: "tool_calls", ToolCalls: []types.LLMToolCall{{ID: "calc", Type: "function", Function: types.FunctionCall{Name: tools.ToolShellExec, Arguments: `{}`}}}}
	} else {
		ch <- types.StreamResponse{ResponseType: types.ResponseTypeAnswer, Content: "2", Done: true, FinishReason: "stop"}
	}
	close(ch)
	return ch, nil
}

type quickFallbackTool struct {
	tools.BaseTool
	calls int
}

func (t *quickFallbackTool) Execute(context.Context, json.RawMessage) (*types.ToolResult, error) {
	t.calls++
	return &types.ToolResult{Success: true, Output: "2"}, nil
}
func TestEmployeeQuickNoMatchFallbackCanCompute(t *testing.T) {
	model := &quickFallbackChat{}
	svc := &sessionService{modelService: &stubModelService{chatModel: model}}
	tool := &quickFallbackTool{BaseTool: tools.NewBaseTool(tools.ToolShellExec, "compute", json.RawMessage(`{"type":"object"}`))}
	bus := event.NewEventBus()
	done := make(chan struct{})
	bus.On(event.EventAgentFinalAnswer, func(_ context.Context, e event.Event) error {
		if e.Data.(event.AgentFinalAnswerData).Done {
			close(done)
		}
		return nil
	})
	cm := &types.ChatManage{PipelineRequest: types.PipelineRequest{SessionID: "s", Query: "1+1", EmployeeAssistant: true}, PipelineState: types.PipelineState{SystemPromptOverride: "evidence-first"}, PipelineContext: types.PipelineContext{EventBus: bus.AsEventBusInterface(), CompletionConfig: &types.AgentConfig{MaxIterations: 4}, CompletionTools: []types.Tool{tool}}}
	svc.handleModelFallback(context.Background(), cm)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("no terminal")
	}
	require.Equal(t, 1, tool.calls)
	require.Equal(t, 2, model.calls)
	require.Contains(t, model.first[0].Content, "evidence-first")
}
