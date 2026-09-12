package session

import (
	"context"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestGovernedToolErrorSSEUsesSameSafeClassification(t *testing.T) {
	for _, code := range []string{"catalog_changed", "invalid_sql"} {
		t.Run(code, func(t *testing.T) {
			ctx := context.Background()
			streams := &terminalStreamManager{}
			h := NewAgentStreamHandler(ctx, "session", "message", "request", 1, time.Time{}, &types.Message{}, streams, event.NewEventBus(), nil)
			err := h.handleToolResult(ctx, event.Event{ID: "tool-result", Data: event.AgentToolResultData{ToolName: "governed_data_query", ToolCallID: "call", Success: false, Error: "private SQL and URL", Data: map[string]interface{}{"governed_error_code": code}}})
			require.NoError(t, err)
			require.Len(t, streams.events, 1)
			evt := streams.events[0]
			status, ok := evt.Data["operationalStatus"].(types.RoleBoundedOperationalStatus)
			require.True(t, ok)
			require.Equal(t, status.SafeSummary, evt.Content)
			require.Equal(t, evt.Content, evt.Data["error"])
			require.NotContains(t, evt.Content, "private")
			require.NotEqual(t, types.OperationalStatusExternalToolUnavailable, status.StatusCode)
			require.False(t, evt.Done, "a recoverable tool failure must not terminate the analysis")
		})
	}
}
