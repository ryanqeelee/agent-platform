package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

const externalSchema = `{"contract_version":"edge-governed-query-v1","enterprise_id":"tenant","edge_node_id":"edge","catalog":{"version":"cat-1","freshness_token":"digest-1"},"source":{"source_id":"retail"}}`
const externalQuery = `{"contract_version":"edge-governed-query-v1","enterprise_id":"tenant","edge_node_id":"edge","catalog":{"version":"cat-1","freshness_token":"digest-1"},"query":{"id":"query-1","rows_returned":1,"truncated":false},"columns":[{"name":"amount","type":"Decimal(30,8)"}],"rows":[{"amount":"9007199254740993.12345678"}],"evidence":{"receipt_sha256":"sha256:receipt"}}`

func testEdgeClient(endpoint string) (*GovernedDataClient, error) {
	return NewGovernedDataClient(types.GovernedEdgeConnection{BaseURL: endpoint, Token: "edge-secret", EnterpriseID: "tenant", EdgeNodeID: "edge", SourceID: "retail"}, func(context.Context) error { return nil })
}

type queryInputStore struct {
	sandbox.SessionFileStore
	session, path string
	data          []byte
	err           error
}

func (s *queryInputStore) WriteSessionWorkspaceFile(ctx context.Context, session, path string, data []byte) error {
	s.session, s.path, s.data = session, path, append([]byte(nil), data...)
	return s.err
}

func TestGovernedDataNativeToolsPreserveIdentityAndExactFile(t *testing.T) {
	var requests []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer edge-secret", r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("X-Tenant-ID"))
		var body map[string]any
		if r.Method == http.MethodPost {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		} else {
			body = map[string]any{}
			for k := range r.URL.Query() {
				body[k] = r.URL.Query().Get(k)
			}
		}
		requests = append(requests, body)
		if r.URL.Path == "/v1/catalog" {
			fmt.Fprint(w, externalSchema)
		} else {
			fmt.Fprint(w, externalQuery)
		}
	}))
	defer server.Close()
	client, err := testEdgeClient(server.URL)
	require.NoError(t, err)
	files := &queryInputStore{}
	tools := NewGovernedDataTools(client, files, "owned-session")
	_, err = tools[1].Execute(context.Background(), json.RawMessage(`{"sql":"SELECT 1"}`))
	require.ErrorContains(t, err, "schema before querying")
	require.Empty(t, requests)
	_, err = tools[0].Execute(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	result, err := tools[1].Execute(context.Background(), json.RawMessage(`{"sql":"SELECT sum(amount) FROM v_sales"}`))
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "retail", requests[1]["source_id"])
	require.Equal(t, "cat-1", requests[1]["catalog_version"])
	require.Equal(t, "digest-1", requests[1]["freshness_token"])
	require.Equal(t, "owned-session", files.session)
	require.Equal(t, externalQuery, string(files.data))
	require.Contains(t, result.Data["next_step"], "Do not transcribe")
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(files.data)), result.Data["input_sha256"])
	require.True(t, strings.HasPrefix(files.path, "/workspace/data/governed-query-"))
	require.Contains(t, result.Output, "9007199254740993.12345678")
	require.NotContains(t, result.Output, "edge-secret")
	require.NotContains(t, string(files.data), "edge-secret")
}

func TestGovernedDataRejectsSameVersionWithDifferentCatalogDigest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Method == http.MethodPost {
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		} else {
			body = map[string]any{}
			for k := range r.URL.Query() {
				body[k] = r.URL.Query().Get(k)
			}
		}
		if r.URL.Path == "/v1/catalog" {
			fmt.Fprint(w, externalSchema)
			return
		}
		require.Equal(t, "retail", body["source_id"])
		require.Equal(t, "cat-1", body["catalog_version"])
		require.Equal(t, "digest-1", body["freshness_token"])
		fmt.Fprint(w, strings.Replace(externalQuery, "digest-1", "digest-2", 1))
	}))
	defer server.Close()

	client, err := testEdgeClient(server.URL)
	require.NoError(t, err)
	tools := NewGovernedDataTools(client, nil, "")
	_, err = tools[0].Execute(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	_, err = tools[1].Execute(context.Background(), json.RawMessage(`{"sql":"SELECT 1"}`))
	require.ErrorContains(t, err, "source or catalog mismatch")
}

func TestGovernedDataRejectsModelAuthorityOverrides(t *testing.T) {
	client, err := testEdgeClient("http://127.0.0.1:1")
	require.NoError(t, err)
	client.catalogVersion = "cat-1"
	client.freshnessToken = "digest-1"
	tools := NewGovernedDataTools(client, nil, "")
	for _, args := range []string{`{"sql":"SELECT 1","tenant_id":"other"}`, `{"sql":"SELECT 1","source_id":"other"}`, `{"sql":"SELECT 1","limit":0}`, `null`, `{} {}`} {
		_, err := tools[1].Execute(context.Background(), json.RawMessage(args))
		require.Error(t, err)
		require.NotContains(t, err.Error(), "request failed")
	}
	_, err = tools[0].Execute(context.Background(), json.RawMessage(`{"tenant_id":"other"}`))
	require.Error(t, err)
}

func TestGovernedDataNeverForwardsCredentialToRedirect(t *testing.T) {
	called := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client, err := testEdgeClient(server.URL)
	require.NoError(t, err)
	_, err = NewGovernedDataTools(client, nil, "")[0].Execute(context.Background(), json.RawMessage(`{}`))
	require.ErrorContains(t, err, "HTTP 307")
	require.False(t, called)
}

func TestGovernedDataCancelInterruptsHTTPAndDoesNotStage(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	client, err := testEdgeClient(server.URL)
	require.NoError(t, err)
	client.catalogVersion = "cat-1"
	client.freshnessToken = "digest-1"
	files := &queryInputStore{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewGovernedDataTools(client, files, "s")[1].Execute(ctx, json.RawMessage(`{"sql":"SELECT 1"}`))
		done <- err
	}()
	<-started
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.Empty(t, files.data)
}

func TestGovernedDataStagingFailureKeepsSuccessfulRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, externalQuery) }))
	defer server.Close()
	client, err := testEdgeClient(server.URL)
	require.NoError(t, err)
	client.catalogVersion = "cat-1"
	client.freshnessToken = "digest-1"
	result, err := NewGovernedDataTools(client, &queryInputStore{err: fmt.Errorf("sandbox unavailable")}, "s")[1].Execute(context.Background(), json.RawMessage(`{"sql":"SELECT 1"}`))
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Contains(t, result.Data, "artifact_error")
	require.NotContains(t, result.Data, "input_file")
	require.Contains(t, result.Output, "9007199254740993.12345678")
}

func TestGovernedDataTransportRequiresHTTPSOutsideLiteralLoopback(t *testing.T) {
	for _, endpoint := range []string{"http://center:8891", "http://example.com", "http://localhost:8891", "http://127.0.0.1.example.com", "https://user:password@example.com"} {
		_, err := testEdgeClient(endpoint)
		require.Error(t, err)
	}
	for _, endpoint := range []string{"https://edge.example.com", "http://127.0.0.1:8891", "http://[::1]:8891", "http://10.92.0.11:8891"} {
		_, err := testEdgeClient(endpoint)
		require.NoError(t, err)
	}
}

func TestGovernedDataRechecksAuthorityBeforeReleasingRows(t *testing.T) {
	allowed := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { allowed = false; fmt.Fprint(w, externalQuery) }))
	defer server.Close()
	client, err := testEdgeClient(server.URL)
	require.NoError(t, err)
	client.authorize = func(context.Context) error {
		if !allowed {
			return ErrGovernedDataAccessDenied
		}
		return nil
	}
	client.catalogVersion, client.freshnessToken = "cat-1", "digest-1"
	files := &queryInputStore{}
	_, err = NewGovernedDataTools(client, files, "s")[1].Execute(context.Background(), json.RawMessage(`{"sql":"SELECT 1"}`))
	require.ErrorIs(t, err, ErrGovernedDataAccessDenied)
	require.Empty(t, files.data)
	_, err = NewGovernedDataTools(client, nil, "s")[0].Execute(context.Background(), json.RawMessage(`{}`))
	require.ErrorIs(t, err, ErrGovernedDataAccessDenied)
}

func TestGovernedDataLargeOutputIsValidPreviewAndKeepsFullReturnedFile(t *testing.T) {
	for _, stage := range []string{"file", "failed", "none"} {
		t.Run(stage, func(t *testing.T) {
			rows := make([]any, 20)
			for i := range rows {
				rows[i] = map[string]any{"amount": "9007199254740993.12345678", "description": strings.Repeat("业务", 200)}
			}
			raw, err := json.Marshal(map[string]any{"contract_version": governedEdgeContract, "enterprise_id": "tenant", "edge_node_id": "edge", "catalog": map[string]any{"version": "cat-1", "freshness_token": "catalog-digest"}, "evidence": map[string]any{"receipt_sha256": "query-digest"}, "query": map[string]any{"id": "query-1", "rows_returned": 20, "truncated": true, "applied_limit": 20}, "rows": rows})
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(raw) }))
			defer server.Close()
			client, err := testEdgeClient(server.URL)
			require.NoError(t, err)
			client.catalogVersion = "cat-1"
			client.freshnessToken = "catalog-digest"
			files := &queryInputStore{}
			var store sandbox.SessionFileStore = files
			if stage == "failed" {
				files.err = fmt.Errorf("unavailable")
			}
			if stage == "none" {
				store = nil
			}
			registry := NewToolRegistry()
			registry.SetMaxToolOutputSize(2000)
			for _, tool := range NewGovernedDataTools(client, store, "session") {
				registry.RegisterTool(tool)
			}
			result, err := registry.ExecuteTool(context.Background(), ToolGovernedDataQuery, json.RawMessage(`{"sql":"SELECT amount FROM v_sales"}`))
			require.NoError(t, err)
			require.LessOrEqual(t, utf8.RuneCountInString(result.Output), 2000)
			var preview map[string]any
			require.NoError(t, json.Unmarshal([]byte(result.Output), &preview))
			require.Equal(t, true, preview["rows_preview_only"])
			require.Equal(t, "query-digest", preview["evidence"].(map[string]any)["receipt_sha256"])
			q := preview["query"].(map[string]any)
			require.Equal(t, "query-1", q["id"])
			require.Equal(t, float64(20), q["rows_returned"])
			require.Equal(t, true, q["truncated"])
			require.Less(t, len(preview["rows"].([]any)), 20)
			require.Equal(t, float64(len(preview["rows"].([]any))), preview["preview_row_count"])
			require.Equal(t, stage == "file", result.Success)
			if stage == "file" {
				require.Equal(t, string(raw), string(files.data))
				require.NotEmpty(t, preview["input_file"])
			} else {
				require.NotEmpty(t, result.Error)
			}
		})
	}
}

func TestGovernedSchemaRegistryBudgetPreservesCompleteFile(t *testing.T) {
	columns := make([]any, 100)
	for i := range columns {
		columns[i] = map[string]any{"name": fmt.Sprintf("field_%d", i), "query_usage": strings.Repeat("原记录不可替代未知不补零", 40)}
	}
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(externalSchema), &schema))
	schema["source"].(map[string]any)["tables"] = []any{map[string]any{"table": "v_wide", "columns": columns, "assumption_notes": []string{"不能推断完整"}, "data_contract": map[string]any{"read_consistency": "not a snapshot"}}}
	raw, err := json.Marshal(schema)
	require.NoError(t, err)
	for _, budget := range []int{DefaultMaxToolOutput, 2000, 32} {
		for _, stage := range []string{"file", "failed", "none"} {
			t.Run(fmt.Sprintf("%d/%s", budget, stage), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(raw) }))
				defer server.Close()
				client, err := testEdgeClient(server.URL)
				require.NoError(t, err)
				client.catalogVersion, client.freshnessToken = "cat-1", "digest-1"
				files := &queryInputStore{}
				var store sandbox.SessionFileStore = files
				if stage == "failed" {
					files.err = fmt.Errorf("internal staging detail")
				}
				if stage == "none" {
					store = nil
				}
				registry := NewToolRegistry()
				registry.SetMaxToolOutputSize(budget)
				registry.RegisterTool(NewGovernedDataTools(client, store, "session")[0])
				result, err := registry.ExecuteTool(context.Background(), ToolGovernedDataSchema, json.RawMessage(`{"table":"v_wide"}`))
				require.NoError(t, err)
				require.True(t, json.Valid([]byte(result.Output)))
				require.LessOrEqual(t, utf8.RuneCountInString(result.Output), budget)
				require.Equal(t, stage == "file" && budget > 32, result.Success)
				if result.Success {
					var envelope map[string]any
					require.NoError(t, json.Unmarshal([]byte(result.Output), &envelope))
					require.Equal(t, "v_wide", envelope["table"])
					require.Equal(t, "cat-1", envelope["catalog"].(map[string]any)["version"])
					require.Equal(t, "digest-1", envelope["catalog"].(map[string]any)["freshness_token"])
					require.Equal(t, schema["source"].(map[string]any)["source_id"], envelope["source"].(map[string]any)["source_id"])
					require.Equal(t, true, envelope["file_contains_complete_schema"])
					require.Equal(t, raw, files.data)
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(raw)), envelope["input_sha256"])
					require.Equal(t, files.path, envelope["input_file"])
					require.Contains(t, envelope["next_step"], "shell_exec")
					require.Contains(t, envelope["next_step"], "query_usage")
					require.Contains(t, envelope["next_step"], "column-name strings")
				} else {
					require.NotEmpty(t, result.Error)
					require.NotContains(t, result.Output, "input_file")
					require.NotContains(t, result.Output, "internal staging detail")
				}
				require.NotContains(t, result.Output, "edge-secret")
			})
		}
	}
}

func TestGovernedSmallSchemaRegistryStaysInlineWithoutSandbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, externalSchema) }))
	defer server.Close()
	client, err := testEdgeClient(server.URL)
	require.NoError(t, err)
	client.catalogVersion, client.freshnessToken = "cat-1", "digest-1"
	registry := NewToolRegistry()
	registry.SetMaxToolOutputSize(2000)
	registry.RegisterTool(NewGovernedDataTools(client, nil, "")[0])
	result, err := registry.ExecuteTool(context.Background(), ToolGovernedDataSchema, json.RawMessage(`{}`))
	require.NoError(t, err)
	require.True(t, result.Success)
	require.JSONEq(t, externalSchema, result.Output)
}

func TestGovernedSchemaNameIndexIsCompleteOrOmittedWithinBudget(t *testing.T) {
	for _, n := range []int{61, 3000} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			tables := make([]any, n)
			expected := make([]string, n)
			for i := range tables {
				expected[i] = fmt.Sprintf("v_store_%04d", i)
				tables[i] = map[string]any{"table": expected[i], "columns": strings.Repeat("constraint", 100)}
			}
			schema := map[string]any{"contract_version": governedEdgeContract, "source": map[string]any{"source_id": "retail", "tables": tables}, "catalog": map[string]any{"version": "cat-1", "freshness_token": "digest-1"}}
			raw, err := json.Marshal(schema)
			require.NoError(t, err)
			files := &queryInputStore{}
			tool := &GovernedDataTool{files: files, sessionID: "session"}
			ctx := WithOutputBudget(context.Background(), 4000)
			result, err := tool.boundedSchemaResult(ctx, schema, raw, nil)
			require.NoError(t, err)
			require.True(t, result.Success)
			require.LessOrEqual(t, utf8.RuneCountInString(result.Output), 4000)
			require.Equal(t, raw, files.data)
			require.Contains(t, result.Data["next_step"], "source.tables")
			require.Contains(t, result.Data["next_step"], "definition")
			if n == 61 {
				require.Equal(t, expected, result.Data["table_names"])
			} else {
				require.NotContains(t, result.Data, "table_names", "never advertise an incomplete table index")
			}
		})
	}
}
