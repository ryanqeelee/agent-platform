package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collectFinalAnswerEvents(t *testing.T, engine *AgentEngine) *[]event.AgentFinalAnswerData {
	t.Helper()
	events := &[]event.AgentFinalAnswerData{}
	engine.eventBus.On(event.EventAgentFinalAnswer, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentFinalAnswerData)
		require.True(t, ok)
		*events = append(*events, data)
		return nil
	})
	return events
}

func doneCount(events []event.AgentFinalAnswerData) int {
	count := 0
	for _, evt := range events {
		if evt.Done {
			count++
		}
	}
	return count
}

func finalizationMessages() []chat.Message {
	return []chat.Message{
		{Role: "system", Content: "system constraints"},
		{Role: "user", Content: "original question marker"},
		{Role: "tool", Content: "verified evidence marker"},
	}
}

func TestFinalAnswerValidTextStreamsThenCompletesOnce(t *testing.T) {
	usage := types.TokenUsage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12}
	model := &mockChat{responses: []mockResponse{{chunks: []types.StreamResponse{
		{ResponseType: types.ResponseTypeAnswer, Content: "verified "},
		{ResponseType: types.ResponseTypeAnswer, Content: "answer", Done: true, Usage: &usage},
	}}}}
	engine := newTestEngine(t, model)
	events := collectFinalAnswerEvents(t, engine)
	state := &types.AgentState{}

	err := engine.streamFinalAnswerToEventBus(context.Background(), "question", finalizationMessages(), state, "sess-1")
	require.NoError(t, err)
	require.Len(t, *events, 3)
	assert.Equal(t, "verified answer", (*events)[0].Content+(*events)[1].Content)
	assert.False(t, (*events)[0].Done)
	assert.False(t, (*events)[1].Done, "provider Done must be withheld until validation")
	assert.True(t, (*events)[2].Done)
	assert.Empty(t, (*events)[2].Content)
	assert.Equal(t, 1, doneCount(*events))
	assert.Equal(t, "verified answer", state.FinalAnswer)
	assert.Equal(t, usage.PromptTokens, state.TurnUsage.PromptTokens)
	assert.Equal(t, usage.CompletionTokens, state.TurnUsage.CompletionTokens)
	assert.Equal(t, usage.TotalTokens, state.TurnUsage.TotalTokens)
	require.Len(t, model.opts, 1)
	assert.Equal(t, "none", model.opts[0].ToolChoice)
	assert.Empty(t, model.opts[0].Tools)
}

func TestFinalAnswerInvalidFirstAttemptRetriesOnceAndPreservesUsageAndTranscript(t *testing.T) {
	firstUsage := types.TokenUsage{PromptTokens: 10, CompletionTokens: 1, TotalTokens: 11}
	secondUsage := types.TokenUsage{PromptTokens: 12, CompletionTokens: 2, TotalTokens: 14}
	model := &mockChat{responses: []mockResponse{
		{chunks: []types.StreamResponse{{Done: true, Usage: &firstUsage}}},
		{chunks: []types.StreamResponse{{ResponseType: types.ResponseTypeAnswer, Content: "partial but useful", Done: true, Usage: &secondUsage}}},
	}}
	engine := newTestEngine(t, model)
	events := collectFinalAnswerEvents(t, engine)
	state := &types.AgentState{}

	err := engine.streamFinalAnswerToEventBus(context.Background(), "question", finalizationMessages(), state, "sess-1")
	require.NoError(t, err)
	assert.Equal(t, 2, model.callCount)
	assert.Equal(t, "partial but useful", state.FinalAnswer)
	assert.Equal(t, 22, state.TurnUsage.PromptTokens)
	assert.Equal(t, 3, state.TurnUsage.CompletionTokens)
	assert.Equal(t, 25, state.TurnUsage.TotalTokens)
	assert.Equal(t, 1, doneCount(*events))
	require.Len(t, model.opts, 2)
	for _, opts := range model.opts {
		assert.Equal(t, "none", opts.ToolChoice)
		assert.Empty(t, opts.Tools)
	}

	require.Len(t, model.calls, 2)
	secondCall := model.calls[1]
	var joined strings.Builder
	for _, message := range secondCall {
		joined.WriteString(message.Content)
	}
	assert.Contains(t, joined.String(), "original question marker")
	assert.Contains(t, joined.String(), "verified evidence marker")
	assert.Contains(t, secondCall[len(secondCall)-1].Content, "tool budget is exhausted")
	assert.Contains(t, secondCall[len(secondCall)-1].Content, "verified partial findings")
}

func TestFinalAnswerToolOnlyRetriesWithoutExecutingTool(t *testing.T) {
	model := &mockChat{responses: []mockResponse{
		{chunks: []types.StreamResponse{{
			ToolCalls: []types.LLMToolCall{{
				ID:       "call-1",
				Function: types.FunctionCall{Name: "forbidden_tool", Arguments: `{}`},
			}},
			Done: true,
		}}},
		{chunks: []types.StreamResponse{{ResponseType: types.ResponseTypeAnswer, Content: "answer only", Done: true}}},
	}}
	engine := newTestEngine(t, model)
	tool := newCountingTool("forbidden_tool")
	engine.toolRegistry = agenttools.NewToolRegistry()
	engine.toolRegistry.RegisterTool(tool)
	state := &types.AgentState{}

	err := engine.streamFinalAnswerToEventBus(context.Background(), "question", finalizationMessages(), state, "sess-1")
	require.NoError(t, err)
	assert.Equal(t, 2, model.callCount)
	assert.Zero(t, tool.calls)
	assert.Equal(t, "answer only", state.FinalAnswer)
}

func TestFinalAnswerRepeatedInvalidReturnsErrorWithoutSuccessfulCompletion(t *testing.T) {
	model := &mockChat{responses: []mockResponse{
		{chunks: []types.StreamResponse{{ResponseType: types.ResponseTypeAnswer, Content: " \n\t", Done: true}}},
		{chunks: []types.StreamResponse{{ResponseType: types.ResponseTypeAnswer, Content: "<think>reasoning only</think>", Done: true}}},
	}}
	engine := newTestEngine(t, model, withMaxIterations(0))
	events := collectFinalAnswerEvents(t, engine)
	state := &types.AgentState{}
	var completions []event.AgentCompleteData
	engine.eventBus.On(event.EventAgentComplete, func(_ context.Context, evt event.Event) error {
		data, ok := evt.Data.(event.AgentCompleteData)
		require.True(t, ok)
		completions = append(completions, data)
		return nil
	})

	_, err := engine.executeLoop(
		context.Background(), state, "question", finalizationMessages(), nil, "sess-1", "msg-1",
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no usable answer after one corrective retry")
	assert.Equal(t, 2, model.callCount)
	assert.Zero(t, doneCount(*events))
	assert.False(t, state.IsComplete)
	assert.Empty(t, state.FinalAnswer)
	require.Len(t, completions, 1)
	assert.Equal(t, "failed", completions[0].Outcome)
}

func TestFinalAnswerTextAndToolCallFailsImmediately(t *testing.T) {
	model := &mockChat{responses: []mockResponse{
		{chunks: []types.StreamResponse{
			{
				ResponseType: types.ResponseTypeAnswer,
				Content:      "I will call a tool",
				ToolCalls: []types.LLMToolCall{{
					ID:       "call-1",
					Function: types.FunctionCall{Name: "forbidden_tool", Arguments: `{}`},
				}},
				Done: true,
			},
		}},
	}}
	engine := newTestEngine(t, model)
	events := collectFinalAnswerEvents(t, engine)
	state := &types.AgentState{}

	err := engine.streamFinalAnswerToEventBus(context.Background(), "question", finalizationMessages(), state, "sess-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool calls together with answer text")
	assert.Equal(t, 1, model.callCount, "mixed output must not be retried after text was streamed")
	assert.Zero(t, doneCount(*events))
	assert.Empty(t, state.FinalAnswer)
}

func TestFinalAnswerCustomOptionsCannotReenableTools(t *testing.T) {
	model := &mockChat{responses: []mockResponse{
		{chunks: []types.StreamResponse{
			{ResponseType: types.ResponseTypeAnswer, Content: "answer", Done: true},
		}},
	}}
	engine := newTestEngine(t, model)
	parallel := true
	thinking := true
	engine.SetCompletionOptions(&chat.ChatOptions{
		Tools:             []chat.Tool{{Type: "function", Function: chat.FunctionDef{Name: "forbidden"}}},
		ToolChoice:        "required",
		ParallelToolCalls: &parallel,
		Thinking:          &thinking,
	})

	err := engine.streamFinalAnswerToEventBus(context.Background(), "question", finalizationMessages(), &types.AgentState{}, "sess-1")
	require.NoError(t, err)
	require.Len(t, model.opts, 1)
	assert.Empty(t, model.opts[0].Tools)
	assert.Equal(t, "none", model.opts[0].ToolChoice)
	assert.Nil(t, model.opts[0].ParallelToolCalls)
	assert.Nil(t, model.opts[0].Thinking)
	assert.Positive(t, model.opts[0].MaxTokens)
	assert.Positive(t, model.opts[0].MaxCompletionTokens)
}

type failingFinalChat struct {
	err   error
	calls int
}

func (m *failingFinalChat) ChatStream(context.Context, []chat.Message, *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	m.calls++
	return nil, m.err
}

func (m *failingFinalChat) Chat(context.Context, []chat.Message, *chat.ChatOptions) (*types.ChatResponse, error) {
	return nil, m.err
}

func (m *failingFinalChat) GetModelName() string { return "failing-model" }
func (m *failingFinalChat) GetModelID() string   { return "failing-id" }

func TestFinalAnswerProviderErrorsAndCancellationAreNotRetried(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "provider error", err: errors.New("provider unavailable")},
		{name: "cancellation", err: context.Canceled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			model := &failingFinalChat{err: tt.err}
			engine := newTestEngine(t, model)
			state := &types.AgentState{}

			err := engine.streamFinalAnswerToEventBus(context.Background(), "question", finalizationMessages(), state, "sess-1")
			require.ErrorIs(t, err, tt.err)
			assert.Equal(t, 1, model.calls)
			assert.Empty(t, state.FinalAnswer)
		})
	}
}

func TestFinalAnswerAlreadyCancelledDoesNotCallProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	model := &mockChat{}
	engine := newTestEngine(t, model)
	err := engine.streamFinalAnswerToEventBus(ctx, "question", finalizationMessages(), &types.AgentState{}, "sess-1")
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, model.callCount)
}

type cancelledFinalStream struct {
	*mockChat
	cancel context.CancelFunc
}

func (m *cancelledFinalStream) ChatStream(ctx context.Context, messages []chat.Message, opts *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	stream, err := m.mockChat.ChatStream(ctx, messages, opts)
	m.cancel()
	return stream, err
}

func TestFinalAnswerCancelledStreamCannotRetryOrComplete(t *testing.T) {
	for _, content := range []string{"", "partial answer"} {
		t.Run(content, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			model := &cancelledFinalStream{mockChat: &mockChat{responses: []mockResponse{
				{chunks: []types.StreamResponse{{Content: content, Done: true}}},
			}}, cancel: cancel}
			engine := newTestEngine(t, model)
			state := &types.AgentState{}
			events := collectFinalAnswerEvents(t, engine)
			err := engine.handleMaxIterations(ctx, "question", finalizationMessages(), state, "sess-1")
			require.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, 1, model.callCount)
			assert.False(t, state.IsComplete)
			assert.Zero(t, doneCount(*events))
		})
	}
}
