package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestBuildToolSpanInputKeepsModelAndResolvedArguments(t *testing.T) {
	tc := types.LLMToolCall{
		ID: "call-1",
		Function: types.FunctionCall{
			Name:      "wiki_read_page",
			Arguments: `{"knowledge_base_id":"kb-real","slugs":["summary/uuid"]}`,
		},
		ModelArguments:     `{"knowledge_base_id":"b1","slugs":["res://0001"]}`,
		ArgumentResolution: "resolved",
	}
	resolved := map[string]any{
		"knowledge_base_id": "kb-real",
		"slugs":             []interface{}{"summary/uuid"},
	}
	got := buildToolSpanInput(tc, resolved, false)

	if !reflect.DeepEqual(got["model_arguments"], map[string]interface{}{
		"knowledge_base_id": "b1",
		"slugs":             []interface{}{"res://0001"},
	}) {
		t.Fatalf("model_arguments = %#v", got["model_arguments"])
	}
	if !reflect.DeepEqual(got["resolved_arguments"], resolved) {
		t.Fatalf("resolved_arguments = %#v", got["resolved_arguments"])
	}
	if got["argument_resolution"] != "resolved" {
		t.Fatalf("argument_resolution = %#v", got["argument_resolution"])
	}
}

func TestBuildToolSpanInputRedactsBothArgumentViews(t *testing.T) {
	tc := types.LLMToolCall{
		ID:                 "call-sensitive",
		Function:           types.FunctionCall{Arguments: `{"sql":"select secret","knowledge_base_id":"kb-real"}`},
		ModelArguments:     `{"sql":"select secret","knowledge_base_id":"b1"}`,
		ArgumentResolution: "resolved",
		UnresolvedHandles:  []string{"d99"},
	}
	got := buildToolSpanInput(tc, map[string]any{"sql": "select secret", "knowledge_base_id": "kb-real"}, true)

	if _, ok := got["model_arguments"]; ok {
		t.Fatal("sensitive model arguments leaked")
	}
	if _, ok := got["resolved_arguments"]; ok {
		t.Fatal("sensitive resolved arguments leaked")
	}
	wantKeys := []string{"knowledge_base_id", "sql"}
	if !reflect.DeepEqual(got["model_arg_keys"], wantKeys) || !reflect.DeepEqual(got["resolved_arg_keys"], wantKeys) {
		t.Fatalf("redacted keys = model:%v resolved:%v", got["model_arg_keys"], got["resolved_arg_keys"])
	}
	if got["unresolved_handle_count"] != 1 {
		t.Fatalf("unresolved_handle_count = %#v", got["unresolved_handle_count"])
	}
}

// TestTruncateForLangfuse verifies rune-aware truncation.
// We specifically check CJK and ASCII inputs to ensure the "…" marker is
// appended exactly once without splitting multi-byte characters.
func TestTruncateForLangfuse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"empty", "", 10, ""},
		{"zero-budget", "abcdef", 0, "abcdef"},
		{"under-budget", "hello", 10, "hello"},
		{"at-budget", "hello", 5, "hello"},
		{"over-budget-ascii", "hello world", 5, "hello…"},
		{"over-budget-cjk", "你好世界测试", 3, "你好世…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateForLangfuse(tc.in, tc.n)
			if got != tc.want {
				t.Fatalf("truncateForLangfuse(%q, %d) = %q; want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}

// TestArgKeysSorted ensures argKeys returns a deterministic (sorted) list so
// Langfuse span inputs diff cleanly across runs.
func TestArgKeysSorted(t *testing.T) {
	args := map[string]any{"zeta": 1, "alpha": 2, "mu": 3}
	got := argKeys(args)
	want := []string{"alpha", "mu", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argKeys order = %v; want %v", got, want)
	}
}

// TestDataKeysSorted mirrors TestArgKeysSorted for tool result Data maps.
func TestDataKeysSorted(t *testing.T) {
	data := map[string]interface{}{"z": nil, "a": nil, "m": nil}
	got := dataKeys(data)
	want := []string{"a", "m", "z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dataKeys order = %v; want %v", got, want)
	}
}

// TestFinishToolSpanNilSafe is a regression check: callers always invoke
// finishToolSpan unconditionally (Langfuse is may-be-disabled) so a nil span
// must not panic.
func TestFinishToolSpanNilSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("finishToolSpan panicked on nil span: %v", r)
		}
	}()
	finishToolSpan(context.Background(), nil, types.ToolCall{}, errors.New("boom"), 123)
	finishToolSpan(context.Background(), nil, types.ToolCall{Result: &types.ToolResult{Success: true}}, nil, 0)
}

func TestGovernedToolSpanIsMetadataOnly(t *testing.T) {
	ctx := types.WithGovernedDataObservability(context.Background())
	tc := types.LLMToolCall{
		ID:             "call-governed",
		Function:       types.FunctionCall{Name: "governed_data_query", Arguments: `{"sql":"SQL_SENTINEL"}`},
		ModelArguments: `{"sql":"SQL_SENTINEL"}`,
	}
	input := buildToolSpanInputForContext(ctx, tc, map[string]any{"sql": "SQL_SENTINEL"}, false)
	output := buildToolSpanOutput(ctx, types.ToolCall{Result: &types.ToolResult{
		Success: true,
		Output:  `{"rows":["ROWS_SENTINEL"]}`,
		Data:    map[string]interface{}{"rows": "ROWS_SENTINEL"},
	}}, 42)
	encoded, err := json.Marshal([]interface{}{input, output})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SQL_SENTINEL", "ROWS_SENTINEL"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("governed tool span leaked %q: %s", secret, encoded)
		}
	}
	for _, metadata := range []string{"args_redacted", "output_len", "duration_ms", "success"} {
		if !strings.Contains(string(encoded), metadata) {
			t.Fatalf("governed tool span lost %q: %s", metadata, encoded)
		}
	}
}

func TestEngineDetectsGovernedToolConfigurationAndHistory(t *testing.T) {
	cases := []struct {
		name    string
		config  *types.AgentConfig
		history []chat.Message
		want    bool
	}{
		{name: "ordinary", config: &types.AgentConfig{AllowedTools: []string{"knowledge_search"}}},
		{name: "configured", config: &types.AgentConfig{AllowedTools: []string{"governed_data_schema"}}, want: true},
		{name: "tool result history", config: &types.AgentConfig{}, history: []chat.Message{{Role: "tool", Name: "governed_data_query", Content: "ROWS_SENTINEL"}}, want: true},
		{name: "tool call history", config: &types.AgentConfig{}, history: []chat.Message{{Role: "assistant", ToolCalls: []chat.ToolCall{{Function: chat.FunctionCall{Name: "governed_data_query"}}}}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := &AgentEngine{config: tc.config}
			if got := engine.requiresGovernedDataObservability(tc.history); got != tc.want {
				t.Fatalf("requiresGovernedDataObservability() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestIterOutcomeString locks in the string labels that surface in Langfuse
// span outputs so dashboards built on those values don't silently break.
func TestIterOutcomeString(t *testing.T) {
	cases := []struct {
		o    iterOutcome
		want string
	}{
		{iterOutcomeNext, "next"},
		{iterOutcomeContinue, "continue"},
		{iterOutcomeBreak, "break"},
		{iterOutcome(42), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.o.String(); got != tc.want {
			t.Errorf("iterOutcome(%d).String() = %q; want %q", tc.o, got, tc.want)
		}
	}
}

// TestTruncateRunesEngine verifies the engine-local rune truncator (separate
// budget from act.go's helper) behaves identically for the common cases.
func TestTruncateRunesEngine(t *testing.T) {
	if got := truncateRunes("abcde", 3); got != "abc…" {
		t.Fatalf("truncateRunes short = %q; want %q", got, "abc…")
	}
	if got := truncateRunes("abc", 10); got != "abc" {
		t.Fatalf("truncateRunes under-budget = %q; want abc", got)
	}
	if got := truncateRunes("", 10); got != "" {
		t.Fatalf("truncateRunes empty = %q; want empty", got)
	}
}
