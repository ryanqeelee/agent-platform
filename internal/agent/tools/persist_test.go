package tools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestShouldOmitRawToolOutput(t *testing.T) {
	if !ShouldOmitRawToolOutput(ToolListKnowledgeChunks, map[string]interface{}{"display_type": "knowledge_chunks_list"}) {
		t.Fatal("structured list_knowledge_chunks output should be omitted")
	}
	if !ShouldOmitRawToolOutput(ToolGrepChunks, map[string]interface{}{"display_type": "grep_results"}) {
		t.Fatal("structured grep output should be omitted")
	}
	if ShouldOmitRawToolOutput("custom_tool", nil) {
		t.Fatal("unknown tools should keep raw output by default")
	}
}

func TestSanitizeToolDataForPersist_knowledgeChunksList(t *testing.T) {
	data := map[string]interface{}{
		"display_type":    "knowledge_chunks_list",
		"knowledge_title": "sample.pdf",
		"fetched_chunks":  50,
		"total_chunks":    282,
		"chunks":          []map[string]interface{}{{"content": "secret"}},
	}
	out := SanitizeToolDataForPersist(data)
	if _, ok := out["chunks"]; ok {
		t.Fatal("chunk bodies should be stripped from persisted tool data")
	}
	if out["fetched_chunks"] != 50 {
		t.Fatalf("summary fields should be kept, got %#v", out["fetched_chunks"])
	}
}

func TestSanitizeAgentStepsForStorage_stripsLargeOutput(t *testing.T) {
	steps := []types.AgentStep{{
		Iteration: 1,
		ToolCalls: []types.ToolCall{{
			ID:   "call-1",
			Name: ToolListKnowledgeChunks,
			Result: &types.ToolResult{
				Success: true,
				Output:  strings.Repeat("x", 10000),
				Data: map[string]interface{}{
					"display_type":    "knowledge_chunks_list",
					"knowledge_title": "sample.pdf",
					"fetched_chunks":  50,
					"total_chunks":    282,
					"chunks":          []map[string]interface{}{{"content": "body"}},
				},
			},
		}},
	}}

	sanitized := SanitizeAgentStepsForStorage(steps)
	result := sanitized[0].ToolCalls[0].Result
	if len(result.Output) >= 10000 {
		t.Fatal("persisted output should be compacted")
	}
	if !strings.Contains(result.Output, "content omitted from history") {
		t.Fatalf("unexpected compact output: %q", result.Output)
	}
	if _, ok := result.Data["chunks"]; ok {
		t.Fatal("chunk bodies should be removed from persisted data")
	}
}

func TestSanitizeToolResultForClient_omitsOutput(t *testing.T) {
	meta := SanitizeToolResultForClient(ToolListKnowledgeChunks, &types.ToolResult{
		Success: true,
		Output:  "<knowledge_chunks>very large</knowledge_chunks>",
		Data: map[string]interface{}{
			"display_type":    "knowledge_chunks_list",
			"knowledge_title": "sample.pdf",
			"fetched_chunks":  1,
			"total_chunks":    1,
		},
	})
	if _, ok := meta["output"]; ok {
		t.Fatal("raw output should not be sent to client metadata")
	}
	if meta["fetched_chunks"] != 1 {
		t.Fatalf("summary metadata should remain, got %#v", meta["fetched_chunks"])
	}
}

func TestFailedToolProjectionRemovesRawErrorsFromClientAndHistory(t *testing.T) {
	const raw = "provider token=secret at http://internal.example stack trace"
	result := &types.ToolResult{
		Success: false,
		Error:   raw,
		Data: map[string]interface{}{
			"display_type":          "web_fetch",
			"error_code":            "FETCH_TIMEOUT",
			"retryable":             true,
			"summary_error_message": raw,
			"results": []map[string]interface{}{{
				"status":        "failed",
				"error_code":    "FETCH_TIMEOUT",
				"error_message": raw,
			}},
		},
		Output: raw,
	}

	meta := SanitizeToolResultForClient("web_fetch", result)
	if strings.Contains(strings.Join([]string{
		StreamContentForToolResult("web_fetch", false, raw, result.Data),
		CompactToolOutputForHistory("web_fetch", result),
	}, " "), raw) {
		t.Fatal("raw tool error reached the employee projection")
	}
	if _, ok := meta["summary_error_message"]; ok {
		t.Fatal("raw web-fetch summary error reached client metadata")
	}
	if strings.Contains(fmt.Sprint(meta), raw) {
		t.Fatal("nested raw web-fetch error reached client metadata")
	}
	if _, ok := meta["output"]; ok {
		t.Fatal("failed tool output reached client metadata")
	}
	status, ok := meta["operationalStatus"].(types.RoleBoundedOperationalStatus)
	if !ok || status.StatusCode != types.OperationalStatusExternalToolUnavailable {
		t.Fatalf("missing stable operational status: %#v", meta["operationalStatus"])
	}
	if meta["error_code"] != "FETCH_TIMEOUT" || meta["retryable"] != true {
		t.Fatalf("safe machine fields should remain: %#v", meta)
	}

	steps := SanitizeAgentStepsForStorage([]types.AgentStep{{
		ToolCalls: []types.ToolCall{{Name: "web_fetch", Result: result}},
	}})
	stored := steps[0].ToolCalls[0].Result
	if strings.Contains(fmt.Sprint(stored), raw) {
		t.Fatal("raw tool error reached persisted history")
	}
	if stored.Output != "" {
		t.Fatal("failed tool output should not be persisted")
	}
}
