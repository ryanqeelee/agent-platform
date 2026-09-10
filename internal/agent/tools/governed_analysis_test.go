package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func testGovernedRunReceipt(t *testing.T, base map[string]any, state string) map[string]any {
	t.Helper()
	data, err := json.Marshal(base)
	require.NoError(t, err)
	var receipt governedAnalysisRunReceipt
	require.NoError(t, json.Unmarshal(data, &receipt))
	receipt.State = state
	receipt.BindingDigest = ""
	canonical, err := canonicalGovernedRunMaterial(receipt)
	require.NoError(t, err)
	receipt.BindingDigest = fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))
	data, err = json.Marshal(receipt)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result))
	return result
}

type centerWireFixture struct {
	URL      string         `json:"url"`
	Request  map[string]any `json:"request"`
	Response map[string]any `json:"response"`
}

func loadCenterWireFixture(t *testing.T, name string) centerWireFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/governed_analysis/" + name + ".json")
	require.NoError(t, err)
	var fixture centerWireFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	return fixture
}

func TestGovernedAnalysisLifecycleUsesCenterRunAndAcceptedPresentation(t *testing.T) {
	start := loadCenterWireFixture(t, "01-runs")
	query := loadCenterWireFixture(t, "02-query")
	finalize := loadCenterWireFixture(t, "03-finalize")
	runID := start.Response["runId"].(string)
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer user-token", r.Header.Get("Authorization"))
		require.Equal(t, "10001", r.Header.Get("X-Tenant-ID"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case start.URL:
			require.Equal(t, "governed-analysis-start/1", body["contractVersion"])
			require.Equal(t, "conversation-finalize", body["conversationId"])
			require.Equal(t, "turn-finalize", body["turnId"])
			require.Equal(t, "分析本周销售", body["question"])
			require.NotContains(t, body, "sourceId")
			_ = json.NewEncoder(w).Encode(start.Response)
		case query.URL:
			require.Equal(t, map[string]any{"contractVersion": "governed-analysis-query-request/1", "runId": runID, "sql": "SELECT 1", "limit": float64(100)}, body)
			_ = json.NewEncoder(w).Encode(query.Response)
		case finalize.URL:
			require.Equal(t, "governed-analysis-finalize/1", body["contractVersion"])
			require.Equal(t, runID, body["runId"])
			_ = json.NewEncoder(w).Encode(finalize.Response)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewGovernedDataClient(server.URL, "user-token", "10001", "")
	require.NoError(t, err)
	require.ErrorContains(t, func() error {
		_, err := NewGovernedDataTools(client, nil, "")[0].Execute(context.Background(), json.RawMessage(`{}`))
		return err
	}(), "not been admitted")
	require.NoError(t, client.Start(context.Background(), "conversation-finalize", "turn-finalize", "分析本周销售"))
	client.sourceID = "retail"
	result, err := NewGovernedDataTools(client, nil, "")[1].Execute(context.Background(), json.RawMessage(`{"sql":"SELECT 1","limit":100}`))
	require.NoError(t, err)
	require.Contains(t, result.Output, "evidence_receipt")
	require.Contains(t, result.Output, "result_semantics")
	require.Contains(t, result.Output, "checks")
	draft := finalize.Request["draft"]
	draftBytes, err := json.Marshal(draft)
	require.NoError(t, err)
	presentation, _, err := client.Finalize(context.Background(), string(draftBytes))
	require.NoError(t, err)
	require.Contains(t, presentation, "9007199254740993.12345678")
	require.Equal(t, []string{start.URL, query.URL, finalize.URL}, paths)
}

func TestGovernedAnalysisCopyRequiresSameAuthenticatedTurn(t *testing.T) {
	source := context.WithValue(context.Background(), types.UserIDContextKey, "user-1")
	source = context.WithValue(source, types.TenantIDContextKey, uint64(10001))
	source = types.WithGovernedDataObservability(source)
	source = types.WithGovernedDataUserCredential(source, "jwt")
	client, err := NewGovernedDataClient("http://127.0.0.1:1", "jwt", "10001", "")
	require.NoError(t, err)
	source = WithGovernedAnalysisClient(source, client)
	same := context.WithValue(context.Background(), types.UserIDContextKey, "user-1")
	same = context.WithValue(same, types.TenantIDContextKey, uint64(10001))
	same = types.WithGovernedDataObservability(same)
	same = types.WithGovernedDataUserCredential(same, "jwt")
	_, ok := GovernedAnalysisClientFromContext(CopyGovernedAnalysisClient(same, source))
	require.True(t, ok)
	other := context.WithValue(same, types.TenantIDContextKey, uint64(10002))
	_, ok = GovernedAnalysisClientFromContext(CopyGovernedAnalysisClient(other, source))
	require.False(t, ok)
}

func TestGovernedAnalysisRejectsMalformedDraftAndTerminatesOnce(t *testing.T) {
	start := loadCenterWireFixture(t, "01-runs")
	runID := start.Response["runId"].(string)
	terminalCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if r.URL.Path == start.URL {
			_ = json.NewEncoder(w).Encode(start.Response)
			return
		}
		require.Equal(t, "/api/governed-data/analysis/terminate", r.URL.Path)
		terminalCalls++
		require.Equal(t, runID, body["runId"])
		require.Equal(t, "failed", body["state"])
		_ = json.NewEncoder(w).Encode(testGovernedRunReceipt(t, start.Response, "failed"))
	}))
	defer server.Close()
	client, err := NewGovernedDataClient(server.URL, "user", "tenant", "")
	require.NoError(t, err)
	require.NoError(t, client.Start(context.Background(), "c", "t", "q"))
	_, _, err = client.Finalize(context.Background(), "plain prose")
	require.ErrorContains(t, err, "AnalysisDraftV1")
	require.NoError(t, client.Terminate(context.Background(), "failed", "invalid_analysis_draft", err.Error()))
	require.NoError(t, client.Terminate(context.Background(), "failed", "duplicate", "ignored"))
	require.Equal(t, 1, terminalCalls)
}

func TestGovernedAnalysisRejectsUnresolvedCenterFact(t *testing.T) {
	_, err := renderGovernedAnalysisAnswer(map[string]any{
		"factManifest": map[string]any{"facts": []any{}},
		"answer":       map[string]any{"summary": map[string]any{"segments": []any{map[string]any{"type": "fact", "ref": map[string]any{"factId": "missing"}}}}},
	})
	require.ErrorContains(t, err, "unresolved")
}

func TestGovernedQueryRejectsInconsistentEvidenceReceipt(t *testing.T) {
	for _, field := range []string{"runId", "queryExecutionId", "queryDigest", "queryStatus", "returnedRowCount", "persistedRowCount", "citableRowEndExclusive", "queryTruncated"} {
		t.Run(field, func(t *testing.T) {
			var payload map[string]any
			require.NoError(t, json.Unmarshal([]byte(externalQuery), &payload))
			receipt := payload["evidence_receipt"].(map[string]any)
			switch field {
			case "returnedRowCount", "persistedRowCount", "citableRowEndExclusive":
				receipt[field] = float64(2)
			case "queryTruncated":
				receipt[field] = true
			default:
				receipt[field] = "mismatch"
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(payload) }))
			defer server.Close()
			client, err := NewGovernedDataClient(server.URL, "user", "tenant", "retail")
			require.NoError(t, err)
			client.runID, client.catalogVersion, client.catalogDigest = "run-1", "cat-1", "digest-1"
			_, err = NewGovernedDataTools(client, nil, "")[1].Execute(context.Background(), json.RawMessage(`{"sql":"SELECT 1"}`))
			require.ErrorContains(t, err, "evidence receipt")
		})
	}
}

func TestGovernedAnalysisTerminationConflictStopsLifecycle(t *testing.T) {
	start := loadCenterWireFixture(t, "01-runs")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == start.URL {
			_ = json.NewEncoder(w).Encode(start.Response)
			return
		}
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()
	client, err := NewGovernedDataClient(server.URL, "user", "tenant", "")
	require.NoError(t, err)
	require.NoError(t, client.Start(context.Background(), "c", "t", "q"))
	require.ErrorContains(t, client.Terminate(context.Background(), "cancelled", "user_cancelled", "stopped"), "HTTP 409")
	require.NoError(t, client.Terminate(context.Background(), "cancelled", "duplicate", "ignored"))
}

func TestGovernedAnalysisHeartbeatFailureCancelsTurn(t *testing.T) {
	start := loadCenterWireFixture(t, "01-runs")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == start.URL {
			json.NewEncoder(w).Encode(start.Response)
			return
		}
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()
	client, err := NewGovernedDataClient(server.URL, "user", "tenant", "")
	require.NoError(t, err)
	require.NoError(t, client.Start(context.Background(), "c", "t", "q"))
	turnCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client.SetTurnCancel(cancel)
	require.Error(t, client.heartbeat(context.Background()))
	client.failHeartbeat()
	require.ErrorIs(t, turnCtx.Err(), context.Canceled)
	require.Error(t, client.LifecycleError())
	client.markTerminal()
}

func TestGovernedAnalysisHeartbeatFailureBeforeCancelAttachmentCancelsTurn(t *testing.T) {
	start := loadCenterWireFixture(t, "01-runs")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == start.URL {
			_ = json.NewEncoder(w).Encode(start.Response)
			return
		}
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()
	client, err := NewGovernedDataClient(server.URL, "user", "tenant", "")
	require.NoError(t, err)
	require.NoError(t, client.Start(context.Background(), "c", "t", "q"))
	require.Error(t, client.heartbeat(context.Background()))
	client.failHeartbeat()

	turnCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client.SetTurnCancel(cancel)
	select {
	case <-turnCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("late turn cancel was not invoked after heartbeat failure")
	}
	client.markTerminal()
}

func TestGovernedRunReceiptMatchesCenterCanonicalOracle(t *testing.T) {
	const catalogDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const scopeDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	tests := []governedAnalysisRunReceipt{
		{ContractVersion: "governed-analysis-run/1", RunID: "run_test", State: "running", CatalogVersion: "cat_test", CatalogDigest: catalogDigest, ScopeDigest: scopeDigest, BindingDigest: "sha256:ae69b6e7c0e2be5637ffe22a8b0bcd282a6bf6a1d7f4a6169493f90d499dd216"},
		{ContractVersion: "governed-analysis-run/1", RunID: "run_<&>中文\u2028\u2029", State: "running", CatalogVersion: "cat_中文<&>", CatalogDigest: catalogDigest, ScopeDigest: scopeDigest, BindingDigest: "sha256:9ef009656607dbf328b4184761d63f0a5945f10e6c6c3639e84506ebfc5462f3"},
		{ContractVersion: "governed-analysis-run/1", RunID: "run_<&>中文\u2028\u2029", State: "completed", CatalogVersion: "cat_中文<&>", CatalogDigest: catalogDigest, ScopeDigest: scopeDigest, BindingDigest: "sha256:1e84d03494bf289be147a1c6146f9ed7ca1f734aec02334e424e82c61b2fdf45"},
		{ContractVersion: "governed-analysis-run/1", RunID: `run_\u2028\u2029`, State: "running", CatalogVersion: "cat_\\\"\n\t\b\f\r", CatalogDigest: catalogDigest, ScopeDigest: scopeDigest, BindingDigest: "sha256:494882ae0402215b8d8e855c5c3fe8ab6541754bc8d669a2f6cc7740c39727ed"},
	}
	for _, receipt := range tests {
		require.True(t, validGovernedRunReceipt(receipt, receipt.State), receipt.RunID)
	}
	canonical, err := canonicalGovernedRunMaterial(tests[3])
	require.NoError(t, err)
	require.Contains(t, string(canonical), `"runId":"run_\\u2028\\u2029"`)
}

func TestGovernedRunReceiptRejectsInvalidAuthorityBinding(t *testing.T) {
	base := governedAnalysisRunReceipt{
		ContractVersion: "governed-analysis-run/1", RunID: "run_test", State: "running", CatalogVersion: "cat_test",
		CatalogDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ScopeDigest:   "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		BindingDigest: "sha256:ae69b6e7c0e2be5637ffe22a8b0bcd282a6bf6a1d7f4a6169493f90d499dd216",
	}
	for _, mutate := range []func(*governedAnalysisRunReceipt){
		func(r *governedAnalysisRunReceipt) { r.CatalogDigest = "sha256:" + r.CatalogDigest },
		func(r *governedAnalysisRunReceipt) { r.ScopeDigest = strings.TrimPrefix(r.ScopeDigest, "sha256:") },
		func(r *governedAnalysisRunReceipt) { r.BindingDigest = "sha256:" + strings.Repeat("0", 64) },
		func(r *governedAnalysisRunReceipt) { r.BindingDigest = "SHA256:" + strings.Repeat("0", 64) },
	} {
		receipt := base
		mutate(&receipt)
		require.False(t, validGovernedRunReceipt(receipt, "running"))
	}
}

func TestGovernedAnalysisRejectsCompletedReceiptFromDifferentScope(t *testing.T) {
	start := loadCenterWireFixture(t, "01-runs")
	finalize := loadCenterWireFixture(t, "03-finalize")
	completed := testGovernedRunReceipt(t, finalize.Response["run"].(map[string]any), "completed")
	completed["scopeDigest"] = "sha256:" + strings.Repeat("c", 64)
	completed = testGovernedRunReceipt(t, completed, "completed")
	response := make(map[string]any, len(finalize.Response))
	for key, value := range finalize.Response {
		response[key] = value
	}
	response["run"] = completed
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == start.URL {
			_ = json.NewEncoder(w).Encode(start.Response)
			return
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	client, err := NewGovernedDataClient(server.URL, "user", "tenant", "")
	require.NoError(t, err)
	require.NoError(t, client.Start(context.Background(), "c", "t", "q"))
	draftBytes, err := json.Marshal(finalize.Request["draft"])
	require.NoError(t, err)
	_, _, err = client.Finalize(context.Background(), string(draftBytes))
	require.ErrorContains(t, err, "does not match completed run")
	require.False(t, client.IsTerminal())
	client.markTerminal()
}

func TestGovernedAnalysisCompletedRunSurvivesLocalPresentationFailure(t *testing.T) {
	start := loadCenterWireFixture(t, "01-runs")
	finalize := loadCenterWireFixture(t, "03-finalize")
	runID := start.Response["runId"].(string)
	var terminalCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case start.URL:
			_ = json.NewEncoder(w).Encode(start.Response)
		case finalize.URL:
			answer := map[string]any{
				"contractVersion": "analytical-answer/1", "runId": runID,
				"factManifest": map[string]any{"facts": []any{}},
				"answer":       map[string]any{"summary": map[string]any{"segments": []any{map[string]any{"type": "fact", "ref": map[string]any{"factId": "missing"}}}}},
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"contractVersion": "governed-analysis-result/1", "run": finalize.Response["run"], "answer": answer})
		case "/api/governed-data/analysis/terminate":
			terminalCalls.Add(1)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := NewGovernedDataClient(server.URL, "user", "tenant", "")
	require.NoError(t, err)
	require.NoError(t, client.Start(context.Background(), "c", "t", "q"))
	draftBytes, err := json.Marshal(finalize.Request["draft"])
	require.NoError(t, err)
	presentation, result, err := client.Finalize(context.Background(), string(draftBytes))
	require.ErrorContains(t, err, "unresolved")
	require.Empty(t, presentation)
	require.NotNil(t, result)
	require.True(t, client.IsTerminal())
	require.NoError(t, client.Terminate(context.Background(), "failed", "presentation_failed", err.Error()))
	require.Zero(t, terminalCalls.Load())
}

func TestGovernedAnalysisConcurrentFinalizeOwnsTerminalHTTP(t *testing.T) {
	start := loadCenterWireFixture(t, "01-runs")
	finalize := loadCenterWireFixture(t, "03-finalize")
	entered := make(chan struct{})
	release := make(chan struct{})
	var finalizeCalls atomic.Int32
	var terminateCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case start.URL:
			_ = json.NewEncoder(w).Encode(start.Response)
		case finalize.URL:
			finalizeCalls.Add(1)
			close(entered)
			<-release
			_ = json.NewEncoder(w).Encode(finalize.Response)
		case "/api/governed-data/analysis/terminate":
			terminateCalls.Add(1)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := NewGovernedDataClient(server.URL, "user", "tenant", "")
	require.NoError(t, err)
	require.NoError(t, client.Start(context.Background(), "c", "t", "q"))
	draftBytes, err := json.Marshal(finalize.Request["draft"])
	require.NoError(t, err)
	var wg sync.WaitGroup
	var finalizeErr, terminateErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _, finalizeErr = client.Finalize(context.Background(), string(draftBytes))
	}()
	<-entered
	wg.Add(1)
	go func() {
		defer wg.Done()
		terminateErr = client.Terminate(context.Background(), "failed", "competing_error", "ignored")
	}()
	close(release)
	wg.Wait()
	require.NoError(t, finalizeErr)
	require.NoError(t, terminateErr)
	require.Equal(t, int32(1), finalizeCalls.Load())
	require.Zero(t, terminateCalls.Load())
}

func TestGovernedAnalysisConcurrentTerminateSendsOneTerminalHTTP(t *testing.T) {
	start := loadCenterWireFixture(t, "01-runs")
	response := testGovernedRunReceipt(t, start.Response, "failed")
	entered := make(chan struct{})
	release := make(chan struct{})
	var terminalCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == start.URL {
			_ = json.NewEncoder(w).Encode(start.Response)
			return
		}
		if terminalCalls.Add(1) == 1 {
			close(entered)
			<-release
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	client, err := NewGovernedDataClient(server.URL, "user", "tenant", "")
	require.NoError(t, err)
	require.NoError(t, client.Start(context.Background(), "c", "t", "q"))
	errs := make(chan error, 2)
	go func() { errs <- client.Terminate(context.Background(), "failed", "one", "first") }()
	<-entered
	go func() { errs <- client.Terminate(context.Background(), "failed", "two", "second") }()
	close(release)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	require.Equal(t, int32(1), terminalCalls.Load())
}

func TestGovernedClaimsRoundTripAndAcceptedInsightRendering(t *testing.T) {
	start := loadCenterWireFixture(t, "claims-start")
	finalize := loadCenterWireFixture(t, "claims-finalize")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case start.URL:
			_ = json.NewEncoder(w).Encode(start.Response)
		case finalize.URL:
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, finalize.Request["draft"], body["draft"])
			_ = json.NewEncoder(w).Encode(finalize.Response)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := NewGovernedDataClient(server.URL, "user-token", "10001", "")
	require.NoError(t, err)
	require.NoError(t, client.Start(context.Background(), "conversation", "turn", "question"))
	draft, err := json.Marshal(finalize.Request["draft"])
	require.NoError(t, err)
	presentation, result, err := client.Finalize(context.Background(), string(draft))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, client.IsTerminal())
	require.Contains(t, presentation, "观察：本周销售额：9007199254740993.12345678")
	require.NotContains(t, presentation, "111元")
}

func TestGovernedCalculationClaimsUseCenterExactValues(t *testing.T) {
	for _, tc := range []struct{ name, exact string }{
		{"claims-difference", "2.30"}, {"claims-relative-change", "0.230000000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile("testdata/governed_analysis/" + tc.name + ".json")
			require.NoError(t, err)
			var fixture struct {
				Draft  map[string]any `json:"draft"`
				Answer map[string]any `json:"answer"`
			}
			require.NoError(t, json.Unmarshal(data, &fixture))
			presentation, err := renderGovernedAnalysisAnswer(fixture.Answer)
			require.NoError(t, err)
			require.Contains(t, presentation, "观察："+tc.exact)
			require.NotContains(t, presentation, "%")
			claim := fixture.Draft["claims"].([]any)[0].(map[string]any)
			segment := claim["segments"].([]any)[0].(map[string]any)
			require.Equal(t, "calculationValue", segment["type"])
			require.Equal(t, "1", segment["calculatorVersion"])
		})
	}
}

func TestGovernedInsightLabelsAndUnresolvedReferences(t *testing.T) {
	fixture := loadCenterWireFixture(t, "claims-finalize")
	answer := fixture.Response["answer"].(map[string]any)
	insight := answer["answer"].(map[string]any)["insights"].([]any)[0].(map[string]any)
	for _, tc := range []struct{ kind, status, label string }{
		{"business_interpretation", "unverified", "解释（未验证）"},
		{"hypothesis", "unverified", "假设（未验证）"},
		{"business_interpretation", "rejected", "解释（已拒绝）"},
	} {
		insight["kind"], insight["verificationStatus"] = tc.kind, tc.status
		text, err := renderGovernedAnalysisAnswer(answer)
		require.NoError(t, err)
		require.Contains(t, text, tc.label)
	}
	insight["claim"] = map[string]any{"segments": []any{map[string]any{"type": "fact", "ref": map[string]any{"factId": "missing"}}}}
	_, err := renderGovernedAnalysisAnswer(answer)
	require.ErrorContains(t, err, "unresolved")
	insight["claim"] = map[string]any{"segments": []any{map[string]any{"type": "calculationValue", "calculatorId": "difference"}}}
	_, err = renderGovernedAnalysisAnswer(answer)
	require.ErrorContains(t, err, "unsupported")
}
