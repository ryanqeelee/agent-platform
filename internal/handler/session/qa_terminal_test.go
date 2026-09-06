package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type terminalMessageService struct {
	interfaces.MessageService
	mu      sync.Mutex
	updates []*types.Message
}

func (s *terminalMessageService) UpdateMessage(_ context.Context, msg *types.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *msg
	s.updates = append(s.updates, &copy)
	return nil
}

func (s *terminalMessageService) IndexMessageToKB(context.Context, string, string, string, string) {}

func (s *terminalMessageService) updateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.updates)
}

type terminalStreamManager struct {
	mu     sync.Mutex
	events []interfaces.StreamEvent
}

func (s *terminalStreamManager) AppendEvent(
	_ context.Context, _, _ string, evt interfaces.StreamEvent,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, evt)
	return nil
}

func (s *terminalStreamManager) GetEvents(
	context.Context, string, string, int,
) ([]interfaces.StreamEvent, int, error) {
	return nil, 0, nil
}

func TestNormalQAErrorPersistsFailureAndTerminatesStream(t *testing.T) {
	ctx := context.Background()
	bus := event.NewEventBus()
	messages := &terminalMessageService{}
	streams := &terminalStreamManager{}
	assistant := &types.Message{ID: "message-1", SessionID: "session-1", Role: "assistant"}
	h := &Handler{messageService: messages}

	streamHandler := NewAgentStreamHandler(
		ctx, "session-1", "message-1", "request-1", 7, time.Time{},
		assistant, streams, bus, nil,
	)
	streamHandler.Subscribe()
	h.registerNormalQATerminalHandlers(&sseStreamContext{
		eventBus: bus, asyncCtx: ctx, assistantMessage: assistant,
	}, &qaRequestContext{
		sessionID: "session-1", session: &types.Session{TenantID: 7},
		assistantMessage: assistant,
	})

	require.NoError(t, bus.Emit(ctx, event.Event{
		ID: "error-1", Type: event.EventError, SessionID: "session-1",
		Data: event.ErrorData{Error: "private provider failure", Stage: "chat_completion_stream"},
	}))

	require.Equal(t, 1, messages.updateCount())
	assert.True(t, assistant.IsCompleted)
	assert.Equal(t, types.OperationalStatusSummary(types.OperationalStatusModelRuntimeUnavailable), assistant.Content)
	require.Len(t, streams.events, 2)
	assert.Equal(t, types.ResponseTypeError, streams.events[0].Type)
	assert.Equal(t, types.ResponseTypeComplete, streams.events[1].Type)
	assert.True(t, streams.events[1].Done)
}

func TestNormalQASuccessCompletesOnlyOnceWhenLateErrorArrives(t *testing.T) {
	ctx := context.Background()
	bus := event.NewEventBus()
	messages := &terminalMessageService{}
	streams := &terminalStreamManager{}
	assistant := &types.Message{ID: "message-1", SessionID: "session-1", Role: "assistant"}
	h := &Handler{messageService: messages}
	streamHandler := NewAgentStreamHandler(
		ctx, "session-1", "message-1", "request-1", 7, time.Time{},
		assistant, streams, bus, nil,
	)
	streamHandler.Subscribe()
	h.registerNormalQATerminalHandlers(&sseStreamContext{
		eventBus: bus, asyncCtx: ctx, assistantMessage: assistant,
	}, &qaRequestContext{
		sessionID: "session-1", session: &types.Session{TenantID: 7},
		assistantMessage: assistant,
	})

	require.NoError(t, bus.Emit(ctx, event.Event{
		ID: "answer-1", Type: event.EventAgentFinalAnswer, SessionID: "session-1",
		Data: event.AgentFinalAnswerData{Content: "answer", Done: true},
	}))
	require.NoError(t, bus.Emit(ctx, event.Event{
		ID: "error-1", Type: event.EventError, SessionID: "session-1",
		Data: event.ErrorData{Error: "late failure", Stage: "chat_completion_stream"},
	}))

	assert.Equal(t, 1, messages.updateCount())
	assert.True(t, assistant.IsCompleted)
	assert.Equal(t, "answer", assistant.Content)
	completeEvents := 0
	for _, evt := range streams.events {
		if evt.Type == types.ResponseTypeComplete {
			completeEvents++
		}
	}
	assert.Equal(t, 1, completeEvents)
}
