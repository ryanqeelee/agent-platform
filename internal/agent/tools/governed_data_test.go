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
	"github.com/stretchr/testify/require"
)

const externalSchema = `{"schema":"GovernedDataSchemaV1","catalog_version":"cat-1","catalog_digest":"digest-1","source":{"source_id":"retail"}}`
const externalQuery = `{"schema":"GovernedDataQueryV1","catalog_version":"cat-1","catalog_digest":"digest-1","result":{"status":"ok","source_id":"retail","rows":[{"amount":"9007199254740993.12345678"}],"row_count":1,"truncated":false}}`

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
		require.Equal(t, "Bearer user-secret", r.Header.Get("Authorization"))
		require.Equal(t, "10001", r.Header.Get("X-Tenant-ID"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		requests = append(requests, body)
		if r.URL.Path == "/api/governed-data/schema" {
			fmt.Fprint(w, externalSchema)
		} else {
			fmt.Fprint(w, externalQuery)
		}
	}))
	defer server.Close()
	client, err := NewGovernedDataClient(server.URL, "user-secret", "10001", "")
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
	require.Equal(t, "digest-1", requests[1]["catalog_digest"])
	require.Equal(t, "owned-session", files.session)
	require.Equal(t, externalQuery, string(files.data))
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(files.data)), result.Data["input_sha256"])
	require.True(t, strings.HasPrefix(files.path, "/workspace/data/governed-query-"))
	require.Contains(t, result.Output, "9007199254740993.12345678")
	require.NotContains(t, result.Output, "user-secret")
	require.NotContains(t, string(files.data), "user-secret")
}

func TestGovernedDataRejectsSameVersionWithDifferentCatalogDigest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if r.URL.Path == "/api/governed-data/schema" {
			fmt.Fprint(w, externalSchema)
			return
		}
		require.Equal(t, "retail", body["source_id"])
		require.Equal(t, "cat-1", body["catalog_version"])
		require.Equal(t, "digest-1", body["catalog_digest"])
		fmt.Fprint(w, `{"schema":"GovernedDataQueryV1","catalog_version":"cat-1","catalog_digest":"digest-2","result":{"status":"ok","source_id":"retail","rows":[],"row_count":0,"truncated":false}}`)
	}))
	defer server.Close()

	client, err := NewGovernedDataClient(server.URL, "user", "tenant", "")
	require.NoError(t, err)
	tools := NewGovernedDataTools(client, nil, "")
	_, err = tools[0].Execute(context.Background(), json.RawMessage(`{}`))
	require.NoError(t, err)
	_, err = tools[1].Execute(context.Background(), json.RawMessage(`{"sql":"SELECT 1"}`))
	require.ErrorContains(t, err, "source or catalog mismatch")
}

func TestGovernedDataRejectsModelAuthorityOverrides(t *testing.T) {
	client, err := NewGovernedDataClient("http://127.0.0.1:1", "user", "tenant", "retail")
	require.NoError(t, err)
	client.catalogVersion = "cat-1"
	client.catalogDigest = "digest-1"
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
	client, err := NewGovernedDataClient(server.URL, "user-secret", "tenant", "retail")
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
	client, err := NewGovernedDataClient(server.URL, "user", "tenant", "retail")
	require.NoError(t, err)
	client.catalogVersion = "cat-1"
	client.catalogDigest = "digest-1"
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
	client, err := NewGovernedDataClient(server.URL, "user", "tenant", "retail")
	require.NoError(t, err)
	client.catalogVersion = "cat-1"
	client.catalogDigest = "digest-1"
	result, err := NewGovernedDataTools(client, &queryInputStore{err: fmt.Errorf("sandbox unavailable")}, "s")[1].Execute(context.Background(), json.RawMessage(`{"sql":"SELECT 1"}`))
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Contains(t, result.Data, "artifact_error")
	require.NotContains(t, result.Data, "input_file")
	require.Contains(t, result.Output, "9007199254740993.12345678")
}

func TestGovernedDataTransportRequiresHTTPSOutsideLiteralLoopback(t *testing.T) {
	for _, endpoint := range []string{"http://center:8891", "http://example.com", "http://localhost:8891", "http://127.0.0.1.example.com", "https://user:password@example.com"} {
		_, err := NewGovernedDataClient(endpoint, "secret", "tenant", "")
		require.Error(t, err)
	}
	for _, endpoint := range []string{"https://center.example.com", "http://127.0.0.1:8891", "http://[::1]:8891"} {
		_, err := NewGovernedDataClient(endpoint, "secret", "tenant", "")
		require.NoError(t, err)
	}
}

func TestGovernedDataAccessUsesProductAdmissionBeforeAnyModelTurn(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		allowed    bool
	}{
		{"enabled", `{"schema":"OperatingAnalysisAvailabilityV1","availability":{"canExchange":true}}`, 200, true},
		{"revoked", `{"schema":"OperatingAnalysisAvailabilityV1","availability":{"canExchange":false}}`, 200, false},
		{"unavailable", `{}`, 503, false},
		{"invalid", `{"availability":{"canExchange":true}}`, 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "GET", r.Method)
				require.Equal(t, "/api/auth/operating-analysis-availability", r.URL.Path)
				require.Equal(t, "Bearer user", r.Header.Get("Authorization"))
				require.Equal(t, "tenant", r.Header.Get("X-Tenant-ID"))
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			client, err := NewGovernedDataClient(server.URL, "user", "tenant", "")
			require.NoError(t, err)
			require.Equal(t, tc.allowed, client.CheckAccess(context.Background()) == nil)
		})
	}
}

func TestGovernedDataLargeOutputIsValidPreviewAndKeepsFullReturnedFile(t *testing.T) {
	for _, stage := range []string{"file", "failed", "none"} {
		t.Run(stage, func(t *testing.T) {
			rows := make([]any, 20)
			for i := range rows {
				rows[i] = map[string]any{"amount": "9007199254740993.12345678", "description": strings.Repeat("业务", 200)}
			}
			raw, err := json.Marshal(map[string]any{"schema": "GovernedDataQueryV1", "catalog_version": "cat-1", "catalog_digest": "catalog-digest", "query_digest": "query-digest", "read_consistency": "live read; not snapshot", "result": map[string]any{"status": "ok", "source_id": "retail", "query_execution_id": "query-1", "rows": rows, "row_count": 20, "truncated": true, "applied_limit": 20}})
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(raw) }))
			defer server.Close()
			client, err := NewGovernedDataClient(server.URL, "user", "tenant", "retail")
			require.NoError(t, err)
			client.catalogVersion = "cat-1"
			client.catalogDigest = "catalog-digest"
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
			require.Equal(t, "query-digest", preview["query_digest"])
			q := preview["result"].(map[string]any)
			require.Equal(t, "query-1", q["query_execution_id"])
			require.Equal(t, float64(20), q["row_count"])
			require.Equal(t, true, q["truncated"])
			require.Less(t, len(q["rows"].([]any)), 20)
			require.Equal(t, float64(len(q["rows"].([]any))), preview["preview_row_count"])
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
	schema["tables"] = []any{map[string]any{"table": "v_wide", "columns": columns, "assumption_notes": []string{"不能推断完整"}, "data_contract": map[string]any{"read_consistency": "not a snapshot"}}}
	raw, err := json.Marshal(schema)
	require.NoError(t, err)
	for _, budget := range []int{DefaultMaxToolOutput, 2000, 32} {
		for _, stage := range []string{"file", "failed", "none"} {
			t.Run(fmt.Sprintf("%d/%s", budget, stage), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(raw) }))
				defer server.Close()
				client, err := NewGovernedDataClient(server.URL, "user-secret", "tenant", "")
				require.NoError(t, err)
				client.catalogVersion, client.catalogDigest = "cat-1", "digest-1"
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
					require.Equal(t, "cat-1", envelope["catalog_version"])
					require.Equal(t, "digest-1", envelope["catalog_digest"])
					require.Equal(t, schema["source"], envelope["source"])
					require.Equal(t, true, envelope["file_contains_complete_schema"])
					require.Equal(t, raw, files.data)
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(raw)), envelope["input_sha256"])
					require.Equal(t, files.path, envelope["input_file"])
					require.Contains(t, envelope["next_step"], "shell_exec")
					require.Contains(t, envelope["next_step"], "query_usage")
				} else {
					require.NotEmpty(t, result.Error)
					require.NotContains(t, result.Output, "input_file")
					require.NotContains(t, result.Output, "internal staging detail")
				}
				require.NotContains(t, result.Output, "user-secret")
			})
		}
	}
}

func TestGovernedSmallSchemaRegistryStaysInlineWithoutSandbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, externalSchema) }))
	defer server.Close()
	client, err := NewGovernedDataClient(server.URL, "user", "tenant", "")
	require.NoError(t, err)
	client.catalogVersion, client.catalogDigest = "cat-1", "digest-1"
	registry := NewToolRegistry()
	registry.SetMaxToolOutputSize(2000)
	registry.RegisterTool(NewGovernedDataTools(client, nil, "")[0])
	result, err := registry.ExecuteTool(context.Background(), ToolGovernedDataSchema, json.RawMessage(`{}`))
	require.NoError(t, err)
	require.True(t, result.Success)
	require.JSONEq(t, externalSchema, result.Output)
}
