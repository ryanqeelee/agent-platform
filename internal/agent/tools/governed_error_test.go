package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

// State x path: known stale catalog / SQL / denial / outage / unknown and
// misleading detail -> model error, live SSE, storage, reloaded history.
func TestGovernedFailureProjectionAcrossPaths(t *testing.T) {
	cases := []struct {
		name               string
		status             int
		detail, code, hint string
	}{
		{"catalog", 422, "Catalog version changed; discover the current Catalog", "catalog_changed", "without a table"},
		{"freshness", 422, "data freshness changed; discover the current Catalog", "catalog_changed", "without a table"},
		{"column", 422, "Code: 47. DB::Exception: secret-sql (UNKNOWN_IDENTIFIER)", "invalid_sql", "metric meaning"},
		{"syntax", 422, "Code: 62. DB::Exception: secret-sql (SYNTAX_ERROR)", "invalid_sql", "alias scope"},
		{"denied", 403, "secret-sql", "access_denied", "Do not retry"},
		{"unavailable", 503, "secret-sql", "service_unavailable", "HTTP 503"},
		{"unknown", 422, "secret-sql", "request_failed", "HTTP 422"},
		{"wrong status", 500, "Catalog version changed; discover the current Catalog", "service_unavailable", "HTTP 500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := newGovernedRequestError(tc.status, tc.detail)
			require.Contains(t, err.Error(), tc.detail)
			require.Contains(t, err.Error(), tc.hint)
			data := GovernedFailureData(err)
			require.Equal(t, tc.code, data["governed_error_code"])
			result := &types.ToolResult{Success: false, Error: err.Error(), Data: data}
			status := ToolFailureOperationalStatus(ToolGovernedDataQuery, data)
			live := StreamContentForToolResult(ToolGovernedDataQuery, false, err.Error(), data)
			require.Equal(t, status.SafeSummary, live)
			require.NotContains(t, live, "secret-sql")
			require.NotEqual(t, types.OperationalStatusExternalToolUnavailable, status.StatusCode)
			meta := SanitizeToolResultForClient(ToolGovernedDataQuery, result)
			require.Equal(t, status, meta["operationalStatus"])
			stored := SanitizeAgentStepsForStorage([]types.AgentStep{{ToolCalls: []types.ToolCall{{Name: ToolGovernedDataQuery, Result: result}}}})
			clean := stored[0].ToolCalls[0].Result
			require.Equal(t, live, clean.Error)
			require.Equal(t, "Error: "+live, CompactToolOutputForHistory(ToolGovernedDataQuery, clean))
			require.Equal(t, tc.code, clean.Data["governed_error_code"])
			require.Equal(t, err.Error(), result.Error, "storage must not overwrite current model diagnostics")
			require.Equal(t, ExternalToolOperationalStatus(), ToolFailureOperationalStatus("web_fetch", data))
		})
	}
	require.Nil(t, GovernedFailureData(errors.New("Catalog version changed; discover the current Catalog")))
	require.Nil(t, GovernedFailureData(context.Canceled))
}

func TestGovernedRequestPreservesTypedErrorWithoutRetry(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(422)
		json.NewEncoder(w).Encode(map[string]string{"detail": "Catalog version changed; discover the current Catalog"})
	}))
	defer server.Close()
	client, err := testEdgeClient(server.URL)
	require.NoError(t, err)
	_, err = client.request(context.Background(), "schema", map[string]any{})
	require.Error(t, err)
	require.Equal(t, "catalog_changed", GovernedFailureData(err)["governed_error_code"])
	require.Equal(t, 1, calls, "do not replay a query across catalog changes")
}
