package chatpipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/modelcontext"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

type computationChat struct {
	replies  [][]types.StreamResponse
	messages [][]chat.Message
	opts     []*chat.ChatOptions
}

func (m *computationChat) Chat(context.Context, []chat.Message, *chat.ChatOptions) (*types.ChatResponse, error) {
	return nil, fmt.Errorf("unexpected nonstreaming call")
}
func (m *computationChat) GetModelName() string { return "test" }
func (m *computationChat) GetModelID() string   { return "test" }
func (m *computationChat) ChatStream(_ context.Context, msgs []chat.Message, opt *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	m.messages = append(m.messages, append([]chat.Message(nil), msgs...))
	m.opts = append(m.opts, opt)
	if len(m.replies) == 0 {
		return nil, fmt.Errorf("model rejected completion")
	}
	ch := make(chan types.StreamResponse, len(m.replies[0]))
	for _, chunk := range m.replies[0] {
		ch <- chunk
	}
	close(ch)
	m.replies = m.replies[1:]
	return ch, nil
}

type computationTool struct {
	tools.BaseTool
	calls int
}

func (t *computationTool) Execute(context.Context, json.RawMessage) (*types.ToolResult, error) {
	t.calls++
	if t.calls == 1 {
		return &types.ToolResult{Success: false, Error: "invalid numeric input"}, nil
	}
	return &types.ToolResult{Success: true, Output: "15353.45 12.01"}, nil
}

func TestToolCompletionPreparedEvidenceAndCorrection(t *testing.T) {
	for _, mode := range []string{"direct", "corrected", "final_error"} {
		t.Run(mode, func(t *testing.T) {
			tool := &computationTool{BaseTool: tools.NewBaseTool(tools.ToolShellExec, "compute", json.RawMessage(`{"type":"object"}`))}
			call := func(id string) []types.StreamResponse {
				return []types.StreamResponse{{ResponseType: types.ResponseTypeAnswer, Done: true, FinishReason: "tool_calls", ToolCalls: []types.LLMToolCall{{ID: id, Type: "function", Function: types.FunctionCall{Name: tools.ToolShellExec, Arguments: `{}`}}}}}
			}
			answer := []types.StreamResponse{{ResponseType: types.ResponseTypeAnswer, Content: `15353.45 <ref id="c1"/>`, Done: true, FinishReason: "stop"}}
			model := &computationChat{replies: [][]types.StreamResponse{answer}}
			maxRounds := 4
			if mode == "corrected" {
				model.replies = [][]types.StreamResponse{call("one"), call("two"), answer}
			}
			if mode == "final_error" {
				model.replies = [][]types.StreamResponse{call("one")}
				maxRounds = 1
			}
			registry := modelcontext.NewRegistry(true)
			registry.RegisterChunk(modelcontext.ChunkReference{ChunkID: "source-chunk", KnowledgeBaseID: "source-kb", DocumentTitle: "Source"})
			messages := registry.EncodeMessages([]chat.Message{{Role: "system", Content: "prepared evidence: scope present; preserve conditions"}, {Role: "user", Content: "calculate from source"}})
			bus := event.NewEventBus()
			done := make(chan struct{})
			var events []event.Event
			for _, kind := range []event.EventType{event.EventAgentFinalAnswer, event.EventError, event.EventAgentComplete, event.EventAgentToolResult} {
				bus.On(kind, func(_ context.Context, e event.Event) error {
					events = append(events, e)
					if e.Type == event.EventError || (e.Type == event.EventAgentFinalAnswer && e.Data.(event.AgentFinalAnswerData).Done) {
						close(done)
					}
					return nil
				})
			}
			cm := &types.ChatManage{PipelineRequest: types.PipelineRequest{SessionID: "s", Query: "calculate"}, PipelineContext: types.PipelineContext{MessageID: "m", EventBus: bus.AsEventBusInterface(), CompletionConfig: &types.AgentConfig{MaxIterations: maxRounds, MaxCompletionTokens: 128, MaxContextTokens: 8192}, CompletionTools: []types.Tool{tool}}}
			require.NoError(t, StartToolCompletion(context.Background(), cm, model, &chat.ChatOptions{TopP: 0.7, Seed: 7, MaxCompletionTokens: 1024}, messages, registry))
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("missing terminal event")
			}
			require.Equal(t, messages, model.messages[0])
			require.Equal(t, 0.7, model.opts[0].TopP)
			require.EqualValues(t, 7, model.opts[0].Seed)
			require.LessOrEqual(t, model.opts[0].MaxCompletionTokens, 128)
			results := 0
			failures := 0
			var text strings.Builder
			for _, e := range events {
				require.NotEqual(t, event.EventAgentComplete, e.Type)
				if e.Type == event.EventAgentFinalAnswer {
					text.WriteString(e.Data.(event.AgentFinalAnswerData).Content)
				}
				if e.Type == event.EventAgentToolResult {
					results++
					if !e.Data.(event.AgentToolResultData).Success {
						failures++
					}
				}
			}
			if mode == "final_error" {
				require.Equal(t, event.EventError, events[len(events)-1].Type)
				return
			}
			require.Contains(t, text.String(), `chunk_id="source-chunk"`)
			if mode == "direct" {
				require.Zero(t, tool.calls)
				require.Len(t, model.messages, 1)
			} else {
				require.Equal(t, 2, tool.calls)
				require.Equal(t, 2, results)
				require.Equal(t, 1, failures)
				require.Len(t, model.messages, 3)
				require.Contains(t, fmt.Sprint(model.messages[1]), "invalid numeric input")
				require.Contains(t, fmt.Sprint(model.messages[2]), "15353.45")
			}
		})
	}
}
