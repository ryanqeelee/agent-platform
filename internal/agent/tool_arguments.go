package agent

import (
	"encoding/json"
	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
)

// Normalize schema-declared values before resolving handles nested inside them.
// Keep the provider payload for replay/diagnostics; incomplete JSON is unchanged.
func (e *AgentEngine) normalizeAndDecodeToolCalls(calls []types.LLMToolCall) {
	for i := range calls {
		if calls[i].ModelArguments == "" {
			calls[i].ModelArguments = calls[i].Function.Arguments
		}
		if e.toolRegistry == nil {
			continue
		}
		if tool, err := e.toolRegistry.GetTool(calls[i].Function.Name); err == nil {
			calls[i].Function.Arguments = string(tools.CastParams(json.RawMessage(calls[i].Function.Arguments), tool.Parameters()))
		}
	}
	e.modelContext.DecodeToolCalls(calls)
}
