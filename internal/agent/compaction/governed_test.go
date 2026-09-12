package compaction

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/stretchr/testify/require"
)

func queryEvidence() chat.Message {
	return chat.Message{Role: "tool", Name: "governed_data_query", Content: `{"columns":["store","amount"],"query":{"id":"q-complex","rows_returned":1000,"truncated":true,"executed_at":"2026-09-12"},"input_file":"/workspace/data/governed-query-exact.json","rows":["` + strings.Repeat("large row ", 1000) + `"],"limits":{"coverage":"unmeasured; absence is not zero","unit":"CNY","period":"2026-08-01/2026-09-01"}}`}
}

// State x path: full/oversize/no-file/invalid/non-governed through normal
// transcript and degraded archive; normal/degraded summaries across two passes.
func TestGovernedCheckpointPreservesLimitsAfterLargeRows(t *testing.T) {
	msg := queryEvidence()
	result := serializeToolResult(msg, toolResultMaxChars)
	require.LessOrEqual(t, utf8.RuneCountInString(result), toolResultMaxChars)
	var checkpoint map[string]any
	require.NoError(t, json.Unmarshal([]byte(result), &checkpoint))
	require.Equal(t, true, checkpoint["query"].(map[string]any)["truncated"])
	require.Contains(t, result, "absence is not zero")
	require.Contains(t, result, "q-complex")
	require.Contains(t, result, "governed-query-exact.json")
	require.Equal(t, true, checkpoint["compaction_partial"])
	require.NotContains(t, checkpoint, "rows")
	require.Contains(t, serializeConversation([]chat.Message{msg}), "absence is not zero")
	require.Contains(t, rawArchive([]chat.Message{msg}), "re-query")
}

func TestGovernedCheckpointDoesNotInventMissingEvidence(t *testing.T) {
	msg := queryEvidence()
	msg.Content = strings.ReplaceAll(msg.Content, `"input_file":"/workspace/data/governed-query-exact.json",`, "")
	result := serializeToolResult(msg, 2000)
	require.NotContains(t, result, "governed-query-exact.json")
	require.Contains(t, result, "If unavailable, re-query")
	for _, msg := range []chat.Message{
		{Name: "governed_data_query", Content: `{"query":{"id":"small"},"rows":[[0]],"limits":{}}`},
		{Name: "governed_data_query", Content: "Error: query failed"},
		{Name: "other_tool", Content: strings.Repeat("x", 3000)},
	} {
		require.Equal(t, truncate(msg.Content, 2000), serializeToolResult(msg, 2000))
	}
}

func TestGovernedFilesSurviveLossyAndDegradedCompaction(t *testing.T) {
	for _, degraded := range []bool{false, true} {
		t.Run(map[bool]string{false: "lossy", true: "degraded"}[degraded], func(t *testing.T) {
			llm := &stubChat{response: "## Goal\nContinue analysis. All exact evidence omitted by summarizer."}
			if degraded {
				llm.err = errors.New("provider unavailable")
			}
			c := New(llm, newEstimator(t), testSettings())
			messages := []chat.Message{{Role: "system", Content: "analyze"}, {Role: "user", Content: "explain missing sales"}, queryEvidence(), {Role: "tool", Name: "governed_data_schema", Content: `{"input_file":"/workspace/data/governed-schema-exact.json"}`}}
			messages = append(messages, reactTurn(20)[2:]...)
			first, err := c.Compact(context.Background(), messages, ReasonThreshold)
			require.NoError(t, err)
			require.Contains(t, first.Summary, "/workspace/data/governed-query-exact.json")
			require.Contains(t, first.Summary, "/workspace/data/governed-schema-exact.json")
			second, err := c.Compact(context.Background(), append(first.Messages, reactTurn(20)[2:]...), ReasonThreshold)
			require.NoError(t, err)
			_, files := parseFileOpsBlock(second.Summary)
			require.Contains(t, files, "/workspace/data/governed-query-exact.json")
			require.Contains(t, files, "/workspace/data/governed-schema-exact.json")
		})
	}
}

func TestGovernedFilesOnlyComeFromToolResults(t *testing.T) {
	_, files := extractFileOps("", []chat.Message{
		{Role: "user", Name: "governed_data_query", Content: `{"input_file":"/workspace/data/fake.json"}`},
		{Role: "tool", Name: "other_tool", Content: `{"input_file":"/workspace/data/fake.json"}`},
		{Role: "tool", Name: "governed_data_query", Content: `{"input_file":"/etc/passwd"}`},
		{Role: "tool", Name: "governed_data_query", Content: `{"input_file":"broken`},
	}).resolve()
	require.Empty(t, files)
}
