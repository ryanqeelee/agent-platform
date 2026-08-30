package capabilityplan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestClientResolvesKnowledgeProcessingPlanPin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/internal/product-base/tenants/7/knowledge-processing-plan-pin", r.URL.Path)
		require.Equal(t, "Bearer service-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"contract_version":"KnowledgeProcessingPlanPinV1",
			"plan_version_id":"plan-v1"
		}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "service-token",
		http:    &http.Client{Timeout: time.Second},
	}
	pin, err := client.ResolveKnowledgeProcessingPlan(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "plan-v1", pin.PlanVersionID)
}

func TestClientResolvesPlatformModelRuntimeSettings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/internal/product-base/tenants/7/model-runtime-settings", r.URL.Path)
		require.Equal(t, "Bearer service-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"contract_version":"PlatformModelRuntimeSettingsV1",
			"scope":{"kind":"enterprise_assigned","product_base_tenant_id":"7"},
			"active_plan":{"contract_version":"AICapabilityPlanV1","version_id":"plan-v1"},
			"request_runtime_refs":{
				"employee_assistant_request_runtime":"assistant-v1",
				"operating_analysis_request_runtime":"analysis-v1"
			}
		}`))
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, token: "service-token", http: &http.Client{Timeout: time.Second}}
	settings, err := client.ResolvePlatformModelRuntimeSettings(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "enterprise_assigned", settings.Scope.Kind)
	require.Equal(t, "plan-v1", settings.ActivePlan.VersionID)
	require.Equal(t, "assistant-v1", settings.RequestRuntimeRefs.EmployeeAssistantRequestRuntime)
}

func TestRingxunPlatformModelRuntimeSettingsIntegration(t *testing.T) {
	baseURL := os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_BASE_URL")
	if baseURL == "" {
		t.Skip("requires the Ringxun capability-plan integration fixture")
	}
	client := &Client{
		baseURL: baseURL,
		token:   os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_SERVICE_TOKEN"),
		http:    &http.Client{Timeout: time.Second},
	}
	settings, err := client.ResolvePlatformModelRuntimeSettings(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "platform_shared", settings.Scope.Kind)
	require.Equal(t, os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_VERSION_ID"), settings.ActivePlan.VersionID)
	require.Equal(t, "assistant-integration", settings.RequestRuntimeRefs.EmployeeAssistantRequestRuntime)
}
