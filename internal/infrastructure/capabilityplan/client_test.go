package capabilityplan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientResolvesRongxunPlanContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/internal/product-base/tenants/7/ai-capability-plan", r.URL.Path)
		require.Equal(t, "Bearer service-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"contract_version":"AICapabilityPlanV1",
			"plan_version_id":"plan-v1",
			"source":"tenant_assignment",
			"enterprise":{
				"service_level":"standard",
				"status":"active",
				"usage":null,
				"quota":null,
				"health":{"status":"unknown"}
			}
		}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "service-token",
		http:    &http.Client{Timeout: time.Second},
	}
	resolution, err := client.Resolve(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "plan-v1", resolution.PlanVersionID)
	require.Equal(t, "standard", resolution.Enterprise.ServiceLevel)
}
