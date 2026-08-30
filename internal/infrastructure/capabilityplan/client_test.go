package capabilityplan

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
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

func TestClientResolvesPlatformRetrievalProcessingSettings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/internal/product-base/tenants/7/retrieval-processing-settings", r.URL.Path)
		require.Equal(t, "Bearer service-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"contract_version":"PlatformRetrievalProcessingSettingsV1",
			"scope":{"kind":"enterprise_assigned","product_base_tenant_id":"7"},
			"active_plan":{"contract_version":"AICapabilityPlanV1","version_id":"plan-v1"},
			"capability_refs":{
				"embedding":"embedding-v1",
				"reranking":"reranking-v1",
				"parsing":"parsing-v1"
			}
		}`))
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, token: "service-token", http: &http.Client{Timeout: time.Second}}
	settings, err := client.ResolvePlatformRetrievalProcessingSettings(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "enterprise_assigned", settings.Scope.Kind)
	require.Equal(t, "plan-v1", settings.ActivePlan.VersionID)
	require.Equal(t, "embedding-v1", settings.CapabilityRefs.Embedding)
}

func TestClientResolvesStrictAssistantScenarioCapabilities(t *testing.T) {
	response := `{
		"contract_version":"AssistantScenarioCapabilityV1",
		"scope":{"kind":"enterprise_assigned","product_base_tenant_id":"7"},
		"capabilities":{"external_search":true,"mcp":false,"tools":true}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/internal/product-base/tenants/7/assistant-scenario-capabilities", r.URL.Path)
		require.Equal(t, "Bearer service-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, token: "service-token", http: &http.Client{Timeout: time.Second}}
	settings, err := client.ResolveAssistantScenarioCapabilities(context.Background(), 7)
	require.NoError(t, err)
	require.True(t, settings.Capabilities.ExternalSearch)
	require.False(t, settings.Capabilities.MCP)

	response = `{
		"contract_version":"AssistantScenarioCapabilityV1",
		"scope":{"kind":"enterprise_assigned","product_base_tenant_id":"7"},
		"capabilities":{"external_search":true,"mcp":false,"tools":true,"unknown":true}
	}`
	_, err = client.ResolveAssistantScenarioCapabilities(context.Background(), 7)
	require.Error(t, err)

	response = `{
		"contract_version":"AssistantScenarioCapabilityV1",
		"scope":{"kind":"enterprise_assigned","product_base_tenant_id":"7"},
		"capabilities":{"external_search":true,"tools":true}
	}`
	_, err = client.ResolveAssistantScenarioCapabilities(context.Background(), 7)
	require.Error(t, err)
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

func TestRingxunPlatformRetrievalProcessingSettingsIntegration(t *testing.T) {
	baseURL := os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_BASE_URL")
	if baseURL == "" {
		t.Skip("requires the Ringxun capability-plan integration fixture")
	}
	client := &Client{
		baseURL: baseURL,
		token:   os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_SERVICE_TOKEN"),
		http:    &http.Client{Timeout: time.Second},
	}
	settings, err := client.ResolvePlatformRetrievalProcessingSettings(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "platform_shared", settings.Scope.Kind)
	require.Equal(t, os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_VERSION_ID"), settings.ActivePlan.VersionID)
	require.Equal(t, "embedding-integration", settings.CapabilityRefs.Embedding)
}

func TestRingxunAssistantScenarioCapabilitiesIntegration(t *testing.T) {
	baseURL := os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_BASE_URL")
	if baseURL == "" {
		t.Skip("requires the Ringxun capability-plan integration fixture")
	}
	client := &Client{
		baseURL: baseURL,
		token:   os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_SERVICE_TOKEN"),
		http:    &http.Client{Timeout: time.Second},
	}
	settings, err := client.ResolveAssistantScenarioCapabilities(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, "platform_shared", settings.Scope.Kind)
	require.Equal(t, "7", settings.Scope.ProductBaseTenantID)
	require.Equal(t, true, settings.Capabilities.ExternalSearch)
	require.Equal(t, false, settings.Capabilities.MCP)
	require.Equal(t, true, settings.Capabilities.Tools)
}

func TestRingxunEnterpriseAdministrationQueueIntegration(t *testing.T) {
	baseURL := os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_BASE_URL")
	if baseURL == "" {
		t.Skip("requires the Ringxun capability-plan integration fixture")
	}
	client := &Client{
		baseURL: baseURL,
		token:   os.Getenv("RINGXUN_CAPABILITY_PLAN_INTEGRATION_SERVICE_TOKEN"),
		http:    &http.Client{Timeout: time.Second},
	}
	projection, err := client.ResolveEnterpriseAdministrationQueue(
		context.Background(),
		7,
		types.EnterpriseAdministrationFacts{
			Role:                                "admin",
			ActiveMemberCount:                   3,
			OperatingAnalysisMissingAccessCount: 2,
		},
	)
	require.NoError(t, err)
	require.Equal(t, "enterprise_assigned", projection.Scope.Kind)
	require.Equal(t, "7", projection.Scope.ProductBaseTenantID)
	require.Equal(t, "integration", projection.Summary.ServiceLevel)
	require.Equal(t, 20, *projection.Summary.MemberQuota)
	require.Equal(t, "operating_analysis_access_gap", projection.Items[0].Code)
}

func TestClientResolvesEnterpriseAdministrationQueue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/internal/product-base/tenants/7/enterprise-administration-queue", r.URL.Path)
		require.Equal(t, "Bearer service-token", r.Header.Get("Authorization"))
		var facts types.EnterpriseAdministrationFacts
		require.NoError(t, json.NewDecoder(r.Body).Decode(&facts))
		require.Equal(t, 3, facts.ActiveMemberCount)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"contract_version":"EnterpriseAdministrationQueueV1",
			"scope":{"kind":"enterprise_assigned","product_base_tenant_id":"7"},
			"as_of":"2026-08-30T00:00:00Z",
			"summary":{"service_level":"retail_agent_enterprise","status":"active","member_quota":20,"health":"healthy"},
			"items":[{"code":"operating_analysis_access_gap","priority":"high","count":2,"target":"members"}]
		}`))
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, token: "service-token", http: &http.Client{Timeout: time.Second}}
	projection, err := client.ResolveEnterpriseAdministrationQueue(context.Background(), 7, types.EnterpriseAdministrationFacts{
		Role: "admin", ActiveMemberCount: 3, OperatingAnalysisMissingAccessCount: 2,
	})
	require.NoError(t, err)
	require.Equal(t, 20, *projection.Summary.MemberQuota)
	require.Equal(t, "operating_analysis_access_gap", projection.Items[0].Code)
}
