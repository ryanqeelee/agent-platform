package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestBuildLangfuseGenerationOutput(t *testing.T) {
	toolCalls := []types.LLMToolCall{{ID: "call_1", Type: "function"}}

	got := buildLangfuseGenerationOutput("", "", "tool_calls", toolCalls)
	want := map[string]interface{}{
		"content":       "",
		"tool_calls":    toolCalls,
		"finish_reason": "tool_calls",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("output without reasoning = %#v; want %#v", got, want)
	}

	got = buildLangfuseGenerationOutput("answer", "thinking", "stop", nil)
	want = map[string]interface{}{
		"content":           "answer",
		"tool_calls":        []types.LLMToolCall(nil),
		"finish_reason":     "stop",
		"reasoning_content": "thinking",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("output with reasoning = %#v; want %#v", got, want)
	}
}

func TestSnapshotLangfuseToolCallsKeepsModelArguments(t *testing.T) {
	providerCalls := []types.LLMToolCall{{
		ID:       "call_1",
		Function: types.FunctionCall{Name: "wiki_read_page", Arguments: `{"slugs":["res://0001"]}`},
	}}
	snapshot := snapshotLangfuseToolCalls(providerCalls)
	providerCalls[0].Function.Arguments = `{"slugs":["summary/uuid"]}`

	if got := snapshot[0].Function.Arguments; got != `{"slugs":["res://0001"]}` {
		t.Fatalf("Langfuse snapshot was mutated to %s", got)
	}
}

func TestBuildLangfuseMessagesReasoningContent(t *testing.T) {
	msgs := buildLangfuseMessages([]Message{
		{Role: "assistant", ReasoningContent: "chain of thought", ToolCalls: []ToolCall{{ID: "tc1"}}},
	})
	if len(msgs) != 1 {
		t.Fatalf("len(messages) = %d; want 1", len(msgs))
	}
	if msgs[0]["reasoning_content"] != "chain of thought" {
		t.Fatalf("reasoning_content = %v; want chain of thought", msgs[0]["reasoning_content"])
	}
}

func TestConvertUsageIncludesPromptCacheCounters(t *testing.T) {
	got := convertUsage(&types.TokenUsage{
		PromptTokens: 1000, CompletionTokens: 50, TotalTokens: 1050,
		CacheReadTokens: 800, CacheWriteTokens: 100, CacheMissTokens: 200,
	})
	if got == nil {
		t.Fatal("convertUsage returned nil")
	}
	if got.CacheRead != 800 || got.CacheWrite != 100 || got.CacheMiss != 200 {
		t.Fatalf("cache usage = read:%d write:%d miss:%d", got.CacheRead, got.CacheWrite, got.CacheMiss)
	}
}

func TestGovernedLangfuseGenerationIsMetadataOnly(t *testing.T) {
	ctx := types.WithGovernedDataObservability(context.Background())
	messages := []Message{{
		Role:             "user",
		Content:          "QUERY_SENTINEL",
		ReasoningContent: "REASONING_SENTINEL",
		ToolCalls: []ToolCall{{
			Function: FunctionCall{Name: "governed_data_query", Arguments: `{"sql":"SQL_SENTINEL"}`},
		}},
	}}
	input := langfuseGenerationInput(ctx, messages)
	output := langfuseGenerationOutput(ctx, "ROWS_SENTINEL", "REASONING_SENTINEL", "stop", []types.LLMToolCall{{
		Function: types.FunctionCall{Name: "governed_data_query", Arguments: `{"credential":"CREDENTIAL_SENTINEL"}`},
	}})
	encoded, err := json.Marshal([]interface{}{input, output})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"QUERY_SENTINEL", "REASONING_SENTINEL", "SQL_SENTINEL", "ROWS_SENTINEL", "CREDENTIAL_SENTINEL"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("governed generation leaked %q: %s", secret, encoded)
		}
	}
	for _, metadata := range []string{"message_count", "content_len", "reasoning_len", "tool_call_count", "finish_reason", "payload_omitted"} {
		if !strings.Contains(string(encoded), metadata) {
			t.Fatalf("governed generation lost %q: %s", metadata, encoded)
		}
	}
	if got := langfuseGenerationError(ctx, fmt.Errorf("QUERY_SENTINEL")); got == nil || strings.Contains(got.Error(), "QUERY_SENTINEL") {
		t.Fatalf("governed generation error was not classified safely: %v", got)
	}
}

func TestOrdinaryLangfuseGenerationKeepsPayload(t *testing.T) {
	ctx := context.Background()
	input, ok := langfuseGenerationInput(ctx, []Message{{Role: "user", Content: "ordinary-query"}}).([]map[string]interface{})
	if !ok || len(input) != 1 || input[0]["content"] != "ordinary-query" {
		t.Fatalf("ordinary generation input changed: %#v", input)
	}
	output, ok := langfuseGenerationOutput(ctx, "ordinary-answer", "ordinary-reasoning", "stop", nil).(map[string]interface{})
	if !ok || output["content"] != "ordinary-answer" || output["reasoning_content"] != "ordinary-reasoning" {
		t.Fatalf("ordinary generation output changed: %#v", output)
	}
}

func TestGovernedTurnDisablesLLMDebugSink(t *testing.T) {
	ctx := types.WithGovernedDataObservability(context.Background())
	if llmDebugAllowed(ctx) {
		t.Fatal("governed turn enabled LLM debug sink")
	}
}
