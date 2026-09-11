package operations

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientForwardsServiceActorAndIdempotencyHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "Bearer service-token", request.Header.Get("Authorization"))
		require.Equal(t, "system-admin-1", request.Header.Get("X-Product-Base-Actor-User-Id"))
		require.Equal(t, "activation-1", request.Header.Get("Idempotency-Key"))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":"active"}`))
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, token: "service-token", http: &http.Client{Timeout: time.Second}}
	response, err := client.Do(context.Background(), http.MethodPut,
		"/api/internal/product-base/operations/enterprise-activations/activation-1",
		"system-admin-1", "activation-1", bytes.NewBufferString(`{"name":"Acme"}`))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
}
