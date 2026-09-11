package chatpipeline

import (
	"context"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func boolPtr(value bool) *bool { return &value }

func TestEmployeeEvidenceStatusReachesCurrentUserMessage(t *testing.T) {
	cases := []struct {
		name  string
		state types.PipelineState
		scope types.SearchTargets
		want  string
	}{
		{name: "not searched", state: types.PipelineState{RetrievalNeeded: boolPtr(false)}, want: `status="not_searched"`},
		{name: "no sources", state: types.PipelineState{RetrievalNeeded: boolPtr(true)}, want: `status="no_sources_in_scope"`},
		{name: "no match", scope: types.SearchTargets{{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb"}}, state: types.PipelineState{RetrievalNeeded: boolPtr(true), RetrievalExecuted: true}, want: `status="no_matching_evidence"`},
		{name: "candidate", scope: types.SearchTargets{{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb"}}, state: types.PipelineState{RetrievalNeeded: boolPtr(true), RetrievalExecuted: true, MergeResult: []*types.SearchResult{{ID: "chunk"}}}, want: `status="candidate_evidence"`},
		{name: "partial", scope: types.SearchTargets{{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb"}}, state: types.PipelineState{RetrievalNeeded: boolPtr(true), RetrievalExecuted: true, RetrievalDegraded: true, MergeResult: []*types.SearchResult{{ID: "chunk"}}}, want: `status="candidate_evidence_partial_search"`},
		{name: "rerank failed", scope: types.SearchTargets{{Type: types.SearchTargetTypeKnowledgeBase, KnowledgeBaseID: "kb"}}, state: types.PipelineState{RetrievalNeeded: boolPtr(true), RetrievalExecuted: true, RerankFailed: true, SearchResult: []*types.SearchResult{{ID: "chunk"}}}, want: `status="candidate_evidence_rerank_failed"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cm := &types.ChatManage{
				PipelineRequest: types.PipelineRequest{EmployeeAssistant: true, SearchTargets: tc.scope, SummaryConfig: types.SummaryConfig{Prompt: "stable-system"}},
				PipelineState:   tc.state,
			}
			cm.UserContent = "current question"
			messages := prepareMessagesWithHistory(cm)
			if strings.Contains(messages[0].Content, "<turn_evidence") {
				t.Fatal("dynamic evidence status must not change the stable system prefix")
			}
			if !strings.Contains(messages[len(messages)-1].Content, tc.want) {
				t.Fatalf("current user message = %q, want %s", messages[len(messages)-1].Content, tc.want)
			}
		})
	}
}

// --- IntoChatMessage tests ---

func TestIntoChatMessage_NoKBRetrieval(t *testing.T) {
	cm := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			Query: "hello world",
		},
		PipelineState: types.PipelineState{
			Intent: types.IntentChitchat,
		},
	}
	plugin := &PluginIntoChatMessage{messageService: nil}
	nextCalled := false
	err := plugin.OnEvent(context.Background(), types.INTO_CHAT_MESSAGE, cm, func() *PluginError {
		nextCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !nextCalled {
		t.Fatal("next() was not called")
	}
	if cm.UserContent != "hello world" {
		t.Errorf("UserContent: got %q, want %q", cm.UserContent, "hello world")
	}
}

func TestIntoChatMessage_WithMergeResults(t *testing.T) {
	cm := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			Query: "test query",
			SummaryConfig: types.SummaryConfig{
				ContextTemplate: "Question: {{query}}\n\nReferences:\n{{contexts}}",
			},
		},
		PipelineState: types.PipelineState{
			MergeResult: []*types.SearchResult{
				{Content: "chunk A content"},
				{Content: "chunk B content"},
			},
		},
	}
	plugin := &PluginIntoChatMessage{messageService: nil}
	nextCalled := false
	err := plugin.OnEvent(context.Background(), types.INTO_CHAT_MESSAGE, cm, func() *PluginError {
		nextCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !nextCalled {
		t.Fatal("next() was not called")
	}
	if cm.UserContent == "" {
		t.Fatal("expected UserContent to be populated")
	}
	if !contains(cm.UserContent, "test query") {
		t.Errorf("UserContent should contain query, got: %s", cm.UserContent)
	}
	if !contains(cm.UserContent, "chunk A content") {
		t.Errorf("UserContent should contain chunk A, got: %s", cm.UserContent)
	}
}

func TestIntoChatMessage_ImageDescriptionAppended(t *testing.T) {
	cm := &types.ChatManage{
		PipelineRequest: types.PipelineRequest{
			Query:                   "what is this?",
			ChatModelSupportsVision: false,
		},
		PipelineState: types.PipelineState{
			Intent:           types.IntentChitchat,
			ImageDescription: "a cat sitting on a mat",
		},
	}
	plugin := &PluginIntoChatMessage{messageService: nil}
	_ = plugin.OnEvent(context.Background(), types.INTO_CHAT_MESSAGE, cm, func() *PluginError {
		return nil
	})
	if !contains(cm.UserContent, "a cat sitting on a mat") {
		t.Errorf("UserContent should contain image description, got: %s", cm.UserContent)
	}
}

// --- PipelineBuilder tests ---

func TestPipelineBuilder_Basic(t *testing.T) {
	pipeline := types.NewPipelineBuilder().
		Add(types.LOAD_HISTORY).
		Add(types.CHAT_COMPLETION_STREAM).
		Build()

	if len(pipeline) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(pipeline))
	}
	if pipeline[0] != types.LOAD_HISTORY {
		t.Errorf("stage 0: got %v, want %v", pipeline[0], types.LOAD_HISTORY)
	}
}

func TestPipelineBuilder_AddIf(t *testing.T) {
	pipeline := types.NewPipelineBuilder().
		Add(types.LOAD_HISTORY).
		AddIf(false, types.QUERY_UNDERSTAND).
		AddIf(true, types.CHAT_COMPLETION_STREAM).
		Build()

	if len(pipeline) != 2 {
		t.Fatalf("expected 2 stages (QUERY_UNDERSTAND skipped), got %d", len(pipeline))
	}
	if pipeline[1] != types.CHAT_COMPLETION_STREAM {
		t.Errorf("stage 1: got %v, want %v", pipeline[1], types.CHAT_COMPLETION_STREAM)
	}
}

func TestPipelineBuilder_Empty(t *testing.T) {
	pipeline := types.NewPipelineBuilder().Build()
	if len(pipeline) != 0 {
		t.Fatalf("expected 0 stages, got %d", len(pipeline))
	}
}

// --- helpers ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsString(s, substr))
}

func containsString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
