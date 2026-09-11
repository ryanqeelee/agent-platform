package chatpipeline

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/agent"
	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/modelcontext"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

// StartToolCompletion reuses the Agent loop only for synthesis. Retrieval and
// message persistence remain owned by the quick-answer pipeline. Messages and
// their registry must be the pair already prepared by that pipeline.
func StartToolCompletion(ctx context.Context, cm *types.ChatManage, model chat.Chat,
	options *chat.ChatOptions, messages []chat.Message, registry *modelcontext.Registry,
) error {
	if cm.EventBus == nil || cm.CompletionConfig == nil || registry == nil {
		return fmt.Errorf("tool completion requires an event bus, runtime config and prepared context")
	}
	bus := event.NewEventBus()
	for _, kind := range []event.EventType{event.EventAgentThought, event.EventAgentToolCall,
		event.EventAgentToolResult, event.EventAgentFinalAnswer, event.EventError} {
		bus.On(kind, func(ctx context.Context, evt event.Event) error {
			return cm.EventBus.Emit(ctx, types.Event{ID: evt.ID, Type: types.EventType(evt.Type), SessionID: evt.SessionID, Data: evt.Data})
		})
	}
	// Do not forward agent.complete: it would replace the RAG references and
	// timeline, and duplicate the quick-answer handler's terminal transition.
	toolRegistry := tools.NewToolRegistry()
	for _, tool := range cm.CompletionTools {
		toolRegistry.RegisterTool(tool)
	}
	engine := agent.NewAgentEngine(cm.CompletionConfig, model, toolRegistry, bus, nil, nil, cm.SessionID, "")
	if engine == nil {
		return fmt.Errorf("cannot initialize tool completion engine")
	}
	engine.SetCompletionOptions(options)
	go func() { _, _ = engine.ExecutePrepared(ctx, cm.SessionID, cm.MessageID, cm.Query, messages, registry) }()
	return nil
}
