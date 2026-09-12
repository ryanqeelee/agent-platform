package compaction

import (
	"encoding/json"
	"unicode/utf8"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/models/chat"
)

// Keep query identity and row-limit semantics ahead of large row payloads.
// This is a lossy model checkpoint, never a replacement for the source file.
func serializeToolResult(msg chat.Message, budget int) string {
	if msg.Name != agenttools.ToolGovernedDataQuery || utf8.RuneCountInString(msg.Content) <= budget {
		return truncate(msg.Content, budget)
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(msg.Content), &raw) != nil || raw == nil {
		return truncate(msg.Content, budget)
	}
	var query map[string]json.RawMessage
	if json.Unmarshal(raw["query"], &query) != nil || query == nil {
		return truncate(msg.Content, budget)
	}
	checkpoint := map[string]any{
		"compaction_partial": true,
		"recovery":           "Read exact input_file for SQL, rows, scope and limits before new claims. If unavailable, re-query; a summary is not complete evidence.",
	}
	identity := map[string]json.RawMessage{}
	for _, key := range []string{"id", "executed_at", "rows_returned", "truncated"} {
		if value, ok := query[key]; ok {
			identity[key] = value
		}
	}
	checkpoint["query"] = identity
	// File references are also mechanically inherited by fileOps across summaries.
	for _, key := range []string{"input_file", "input_sha256", "rows_preview_only", "file_contains_all_returned_rows"} {
		if value, ok := raw[key]; ok {
			checkpoint[key] = value
		}
	}
	encoded, _ := json.Marshal(checkpoint)
	if utf8.RuneCount(encoded) > budget {
		return "Governed query evidence omitted by compaction budget. Reopen the recorded governed-query file or re-query before using its figures, scope or completeness."
	}
	// Add whole fields only. Never cut JSON halfway through rows or a limitation.
	for _, key := range []string{"limits", "columns", "rows", "catalog", "evidence"} {
		if value, ok := raw[key]; ok {
			checkpoint[key] = value
			candidate, _ := json.Marshal(checkpoint)
			if utf8.RuneCount(candidate) <= budget {
				encoded = candidate
			} else {
				delete(checkpoint, key)
			}
		}
	}
	return string(encoded)
}
