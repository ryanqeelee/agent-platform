package session

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestNativeDataAnswerStreamsBeforeCompletion(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "user")
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(7))
	ctx = types.WithGovernedDataObservability(types.WithGovernedDataUserCredential(ctx, "jwt"))
	bus := event.NewEventBus()
	streams := &terminalStreamManager{}
	assistant := &types.Message{ID: "message", SessionID: "session", Role: "assistant"}
	h := NewAgentStreamHandler(ctx, "session", "message", "request", 7, time.Time{}, assistant, streams, bus, nil)
	h.Subscribe()
	answer := "2026-09-10 净销售额 1237352.64 元。"
	require.NoError(t, bus.Emit(ctx, event.Event{ID: "answer", Type: event.EventAgentFinalAnswer, Data: event.AgentFinalAnswerData{Content: answer, Done: true}}))
	require.NotEmpty(t, streams.events)
	require.Equal(t, types.ResponseTypeAnswer, streams.events[0].Type)
	require.Equal(t, answer, streams.events[0].Content)
	require.NoError(t, bus.Emit(ctx, event.Event{ID: "complete", Type: event.EventAgentComplete, Data: event.AgentCompleteData{Outcome: "completed", MessageID: "message", FinalAnswer: answer}}))
	require.Equal(t, answer, assistant.Content)
	require.True(t, assistant.IsCompleted)
	require.Nil(t, assistant.ExecutionContext.GovernedAnalysisRun)
	require.Equal(t, types.ResponseTypeComplete, streams.events[len(streams.events)-1].Type)
}

func TestNativeDataErrorOwnsTerminalBeforeLateCompletion(t *testing.T) {
	ctx := types.WithGovernedDataObservability(context.Background())
	bus := event.NewEventBus()
	streams := &terminalStreamManager{}
	assistant := &types.Message{ID: "message", SessionID: "session", Role: "assistant"}
	h := NewAgentStreamHandler(ctx, "session", "message", "request", 7, time.Time{}, assistant, streams, bus, nil)
	h.Subscribe()
	require.NoError(t, bus.Emit(ctx, event.Event{ID: "error", Type: event.EventError, Data: event.ErrorData{Error: "query unavailable", Stage: "agent"}}))
	count := len(streams.events)
	require.NotZero(t, count)
	require.Equal(t, types.ResponseTypeError, streams.events[0].Type)
	require.NoError(t, bus.Emit(ctx, event.Event{ID: "complete", Type: event.EventAgentComplete, Data: event.AgentCompleteData{Outcome: "failed", MessageID: "message", FinalAnswer: "must not replace error"}}))
	require.Len(t, streams.events, count)
	require.NotContains(t, assistant.Content, "must not replace error")
}
