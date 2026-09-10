package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func governedTestReceipt(runID, state string) map[string]any {
	material := fmt.Sprintf(`{"catalogDigest":"%s","catalogVersion":"cat","contractVersion":"governed-analysis-run/1","runId":"%s","scopeDigest":"%s","state":"%s"}`,
		strings.Repeat("a", 64), runID, "sha256:"+strings.Repeat("b", 64), state)
	return map[string]any{
		"contractVersion": "governed-analysis-run/1", "runId": runID, "state": state,
		"catalogVersion": "cat", "catalogDigest": strings.Repeat("a", 64),
		"scopeDigest":   "sha256:" + strings.Repeat("b", 64),
		"bindingDigest": fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(material))),
	}
}

func governedTestAnswer(runID, text string) map[string]any {
	return map[string]any{
		"contractVersion": "analytical-answer/1", "runId": runID,
		"factManifest": map[string]any{"facts": []any{}},
		"answer":       map[string]any{"summary": map[string]any{"segments": []any{map[string]any{"type": "text", "text": text}}}},
	}
}

func governedHandlerContext(t *testing.T, serverURL string) (context.Context, *agenttools.GovernedDataClient) {
	t.Helper()
	ctx := context.WithValue(context.Background(), types.UserIDContextKey, "user")
	ctx = context.WithValue(ctx, types.TenantIDContextKey, uint64(7))
	ctx = types.WithGovernedDataObservability(ctx)
	ctx = types.WithGovernedDataUserCredential(ctx, "jwt")
	client, err := agenttools.NewGovernedDataClient(serverURL, "jwt", "7", "")
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx, "session-1", "message-1", "question"))
	return agenttools.WithGovernedAnalysisClient(ctx, client), client
}

func TestGovernedCompleteFinalizesBeforeFrontendCompleteAndSuppressesDraft(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	runID := "run-1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		switch r.URL.Path {
		case "/api/governed-data/runs":
			json.NewEncoder(w).Encode(governedTestReceipt(runID, "running"))
		case "/api/governed-data/analysis/finalize":
			json.NewEncoder(w).Encode(map[string]any{"contractVersion": "governed-analysis-result/1", "run": governedTestReceipt(runID, "completed"), "answer": governedTestAnswer(runID, "Center accepted")})
		}
	}))
	defer server.Close()
	ctx, _ := governedHandlerContext(t, server.URL)
	bus := event.NewEventBus()
	streams := &terminalStreamManager{}
	assistant := &types.Message{ID: "message-1", SessionID: "session-1", Role: "assistant"}
	h := NewAgentStreamHandler(ctx, "session-1", "message-1", "request", 7, time.Time{}, assistant, streams, bus, nil)
	h.Subscribe()
	draft := `{"contractVersion":"analysis-draft/1","summary":{"text":"draft","evidence":[]}}`
	require.NoError(t, bus.Emit(ctx, event.Event{ID: "draft", Type: event.EventAgentFinalAnswer, Data: event.AgentFinalAnswerData{Content: draft, Done: true}}))
	require.Empty(t, streams.events)
	require.NoError(t, bus.Emit(ctx, event.Event{ID: "complete", Type: event.EventAgentComplete, Data: event.AgentCompleteData{Outcome: "completed", MessageID: "message-1", FinalAnswer: draft}}))
	require.Equal(t, "Center accepted", assistant.Content)
	require.NotNil(t, assistant.ExecutionContext.GovernedAnalysisRun)
	require.Equal(t, types.ResponseTypeAnswer, streams.events[0].Type)
	require.Equal(t, types.ResponseTypeComplete, streams.events[len(streams.events)-1].Type)
	mu.Lock()
	require.Equal(t, []string{"/api/governed-data/runs", "/api/governed-data/analysis/finalize"}, paths)
	mu.Unlock()
}

func TestGovernedFailedCompleteTerminatesWithoutSuccess(t *testing.T) {
	runID := "run-1"
	terminal := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/governed-data/runs" {
			json.NewEncoder(w).Encode(governedTestReceipt(runID, "running"))
			return
		}
		terminal++
		json.NewEncoder(w).Encode(governedTestReceipt(runID, "failed"))
	}))
	defer server.Close()
	ctx, _ := governedHandlerContext(t, server.URL)
	bus := event.NewEventBus()
	streams := &terminalStreamManager{}
	assistant := &types.Message{ID: "message-1", SessionID: "session-1", Role: "assistant"}
	h := NewAgentStreamHandler(ctx, "session-1", "message-1", "request", 7, time.Time{}, assistant, streams, bus, nil)
	h.Subscribe()
	require.NoError(t, bus.Emit(ctx, event.Event{ID: "complete", Type: event.EventAgentComplete, Data: event.AgentCompleteData{Outcome: "failed", MessageID: "message-1", AgentSteps: []types.AgentStep{{Iteration: 1}}}}))
	require.Equal(t, 1, terminal)
	require.Len(t, assistant.AgentSteps, 1)
	require.Len(t, streams.events, 2)
	require.Equal(t, types.ResponseTypeError, streams.events[0].Type)
	require.Equal(t, types.ResponseTypeComplete, streams.events[1].Type)
}

func TestGovernedCancelledCompleteMapsToCenterCancelled(t *testing.T) {
	runID := "run-1"
	var terminalState string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/governed-data/runs" {
			_ = json.NewEncoder(w).Encode(governedTestReceipt(runID, "running"))
			return
		}
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		terminalState, _ = body["state"].(string)
		_ = json.NewEncoder(w).Encode(governedTestReceipt(runID, terminalState))
	}))
	defer server.Close()
	ctx, client := governedHandlerContext(t, server.URL)
	bus := event.NewEventBus()
	streams := &terminalStreamManager{}
	assistant := &types.Message{ID: "message-1", SessionID: "session-1", Role: "assistant"}
	h := NewAgentStreamHandler(ctx, "session-1", "message-1", "request", 7, time.Time{}, assistant, streams, bus, nil)
	h.Subscribe()
	require.NoError(t, bus.Emit(ctx, event.Event{Type: event.EventAgentComplete, Data: event.AgentCompleteData{Outcome: "cancelled", MessageID: "message-1"}}))
	require.Equal(t, "cancelled", terminalState)
	require.True(t, client.IsTerminal())
	require.Empty(t, streams.events)
}

func TestGovernedCenterCompleteLocalRenderFailureRetainsProvenance(t *testing.T) {
	runID := "run-1"
	var terminateCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/governed-data/runs":
			_ = json.NewEncoder(w).Encode(governedTestReceipt(runID, "running"))
		case "/api/governed-data/analysis/finalize":
			answer := governedTestAnswer(runID, "")
			answer["answer"] = map[string]any{"summary": map[string]any{"segments": []any{map[string]any{"type": "fact", "ref": map[string]any{"factId": "missing"}}}}}
			_ = json.NewEncoder(w).Encode(map[string]any{"contractVersion": "governed-analysis-result/1", "run": governedTestReceipt(runID, "completed"), "answer": answer})
		case "/api/governed-data/analysis/terminate":
			terminateCalls.Add(1)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	ctx, client := governedHandlerContext(t, server.URL)
	bus := event.NewEventBus()
	streams := &terminalStreamManager{}
	assistant := &types.Message{ID: "message-1", SessionID: "session-1", Role: "assistant"}
	h := NewAgentStreamHandler(ctx, "session-1", "message-1", "request", 7, time.Time{}, assistant, streams, bus, nil)
	h.Subscribe()
	draft := `{"contractVersion":"analysis-draft/1","summary":{"text":"draft","evidence":[]}}`
	require.NoError(t, bus.Emit(ctx, event.Event{Type: event.EventAgentComplete, Data: event.AgentCompleteData{Outcome: "completed", MessageID: "message-1", FinalAnswer: draft}}))
	require.True(t, client.IsTerminal())
	require.Zero(t, terminateCalls.Load())
	require.Equal(t, runID, assistant.ExecutionContext.GovernedAnalysisRun.RunID)
	require.Len(t, streams.events, 2)
	require.Equal(t, types.ResponseTypeError, streams.events[0].Type)
	require.Equal(t, types.ResponseTypeComplete, streams.events[1].Type)
}

func TestGovernedConcurrentCompleteErrorCancelClaimsOneTerminal(t *testing.T) {
	runID := "run-1"
	entered := make(chan struct{})
	release := make(chan struct{})
	var finalizeCalls atomic.Int32
	var terminateCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/governed-data/runs":
			_ = json.NewEncoder(w).Encode(governedTestReceipt(runID, "running"))
		case "/api/governed-data/analysis/finalize":
			finalizeCalls.Add(1)
			close(entered)
			<-release
			_ = json.NewEncoder(w).Encode(map[string]any{"contractVersion": "governed-analysis-result/1", "run": governedTestReceipt(runID, "completed"), "answer": governedTestAnswer(runID, "accepted")})
		case "/api/governed-data/analysis/terminate":
			terminateCalls.Add(1)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	ctx, _ := governedHandlerContext(t, server.URL)
	streams := &terminalStreamManager{}
	assistant := &types.Message{ID: "message-1", SessionID: "session-1", Role: "assistant"}
	h := NewAgentStreamHandler(ctx, "session-1", "message-1", "request", 7, time.Time{}, assistant, streams, event.NewEventBus(), nil)
	draft := `{"contractVersion":"analysis-draft/1","summary":{"text":"draft","evidence":[]}}`
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		require.NoError(t, h.handleComplete(ctx, event.Event{Type: event.EventAgentComplete, Data: event.AgentCompleteData{Outcome: "completed", MessageID: "message-1", FinalAnswer: draft}}))
	}()
	<-entered
	wg.Add(2)
	go func() {
		defer wg.Done()
		require.NoError(t, h.handleError(ctx, event.Event{Type: event.EventError, Data: event.ErrorData{Error: "late", Stage: "late_error"}}))
	}()
	go func() {
		defer wg.Done()
		require.NoError(t, h.handleComplete(ctx, event.Event{Type: event.EventAgentComplete, Data: event.AgentCompleteData{Outcome: "cancelled", MessageID: "message-1"}}))
	}()
	close(release)
	wg.Wait()
	require.Equal(t, int32(1), finalizeCalls.Load())
	require.Zero(t, terminateCalls.Load())
	require.Equal(t, "accepted", assistant.Content)
	streams.mu.Lock()
	defer streams.mu.Unlock()
	completes := 0
	for _, evt := range streams.events {
		if evt.Type == types.ResponseTypeComplete {
			completes++
		}
	}
	require.Equal(t, 1, completes)
}
