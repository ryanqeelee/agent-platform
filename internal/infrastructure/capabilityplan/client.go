package capabilityplan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClientFromEnv() interfaces.AICapabilityPlanResolver {
	return newClientFromEnv()
}

func NewKnowledgeProcessingPlanResolverFromEnv() interfaces.KnowledgeProcessingPlanResolver {
	return newClientFromEnv()
}

func NewPlatformModelRuntimeSettingsResolverFromEnv() interfaces.PlatformModelRuntimeSettingsResolver {
	return newClientFromEnv()
}

func NewPlatformRetrievalProcessingSettingsResolverFromEnv() interfaces.PlatformRetrievalProcessingSettingsResolver {
	return newClientFromEnv()
}

func NewAssistantScenarioCapabilityResolverFromEnv() interfaces.AssistantScenarioCapabilityResolver {
	return newClientFromEnv()
}

func NewEnterpriseAdministrationQueueResolverFromEnv() interfaces.EnterpriseAdministrationQueueResolver {
	return newClientFromEnv()
}

func newClientFromEnv() *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("RINGXUN_CAPABILITY_PLAN_BASE_URL")), "/"),
		token:   strings.TrimSpace(os.Getenv("RINGXUN_CAPABILITY_PLAN_SERVICE_TOKEN")),
		http:    &http.Client{Timeout: 3 * time.Second},
	}
}

func (c *Client) ResolveKnowledgeProcessingPlan(
	ctx context.Context,
	tenantID uint64,
) (*types.KnowledgeProcessingPlanPin, error) {
	var pin types.KnowledgeProcessingPlanPin
	if err := c.get(ctx, tenantID, "knowledge-processing-plan-pin", &pin); err != nil ||
		pin.ContractVersion != "KnowledgeProcessingPlanPinV1" ||
		strings.TrimSpace(pin.PlanVersionID) == "" ||
		len(pin.PlanVersionID) > 128 ||
		pin.PlanVersionID != strings.TrimSpace(pin.PlanVersionID) {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	return &pin, nil
}

func (c *Client) Resolve(ctx context.Context, tenantID uint64) (*types.AICapabilityPlanResolution, error) {
	var resolution types.AICapabilityPlanResolution
	if err := c.get(ctx, tenantID, "ai-capability-plan", &resolution); err != nil {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	if resolution.ContractVersion != "AICapabilityPlanV1" ||
		strings.TrimSpace(resolution.PlanVersionID) == "" ||
		len(resolution.PlanVersionID) > 128 ||
		resolution.PlanVersionID != strings.TrimSpace(resolution.PlanVersionID) ||
		(resolution.Source != "tenant_assignment" && resolution.Source != "platform_default") ||
		strings.TrimSpace(resolution.Enterprise.ServiceLevel) == "" ||
		strings.TrimSpace(resolution.Enterprise.Status) == "" ||
		strings.TrimSpace(resolution.Enterprise.Health.Status) == "" {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	return &resolution, nil
}

func (c *Client) ResolvePlatformModelRuntimeSettings(
	ctx context.Context,
	tenantID uint64,
) (*types.PlatformModelRuntimeSettings, error) {
	var settings types.PlatformModelRuntimeSettings
	if err := c.get(ctx, tenantID, "model-runtime-settings", &settings); err != nil ||
		settings.ContractVersion != "PlatformModelRuntimeSettingsV1" ||
		(settings.Scope.Kind != "platform_shared" && settings.Scope.Kind != "enterprise_assigned") ||
		settings.Scope.ProductBaseTenantID != strconv.FormatUint(tenantID, 10) ||
		settings.ActivePlan.ContractVersion != "AICapabilityPlanV1" ||
		strings.TrimSpace(settings.ActivePlan.VersionID) == "" ||
		strings.TrimSpace(settings.RequestRuntimeRefs.EmployeeAssistantRequestRuntime) == "" ||
		strings.TrimSpace(settings.RequestRuntimeRefs.OperatingAnalysisRequestRuntime) == "" {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	return &settings, nil
}

func (c *Client) ResolvePlatformRetrievalProcessingSettings(
	ctx context.Context,
	tenantID uint64,
) (*types.PlatformRetrievalProcessingSettings, error) {
	var settings types.PlatformRetrievalProcessingSettings
	if err := c.get(ctx, tenantID, "retrieval-processing-settings", &settings); err != nil ||
		settings.ContractVersion != "PlatformRetrievalProcessingSettingsV1" ||
		(settings.Scope.Kind != "platform_shared" && settings.Scope.Kind != "enterprise_assigned") ||
		settings.Scope.ProductBaseTenantID != strconv.FormatUint(tenantID, 10) ||
		settings.ActivePlan.ContractVersion != "AICapabilityPlanV1" ||
		strings.TrimSpace(settings.ActivePlan.VersionID) == "" ||
		strings.TrimSpace(settings.CapabilityRefs.Embedding) == "" ||
		strings.TrimSpace(settings.CapabilityRefs.Reranking) == "" ||
		strings.TrimSpace(settings.CapabilityRefs.Parsing) == "" {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	return &settings, nil
}

func (c *Client) ResolveAssistantScenarioCapabilities(
	ctx context.Context,
	tenantID uint64,
) (*types.AssistantScenarioCapabilitySettings, error) {
	var payload struct {
		ContractVersion string                      `json:"contract_version"`
		Scope           types.PlatformSettingsScope `json:"scope"`
		Capabilities    struct {
			ExternalSearch *bool `json:"external_search"`
			MCP            *bool `json:"mcp"`
			Tools          *bool `json:"tools"`
		} `json:"capabilities"`
	}
	if err := c.get(ctx, tenantID, "assistant-scenario-capabilities", &payload); err != nil ||
		payload.ContractVersion != "AssistantScenarioCapabilityV1" ||
		(payload.Scope.Kind != "platform_shared" && payload.Scope.Kind != "enterprise_assigned") ||
		payload.Scope.ProductBaseTenantID != strconv.FormatUint(tenantID, 10) ||
		payload.Capabilities.ExternalSearch == nil ||
		payload.Capabilities.MCP == nil ||
		payload.Capabilities.Tools == nil {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	return &types.AssistantScenarioCapabilitySettings{
		ContractVersion: payload.ContractVersion,
		Scope:           payload.Scope,
		Capabilities: types.AssistantScenarioCapabilities{
			ExternalSearch: *payload.Capabilities.ExternalSearch,
			MCP:            *payload.Capabilities.MCP,
			Tools:          *payload.Capabilities.Tools,
		},
	}, nil
}

func (c *Client) ResolveEnterpriseAdministrationQueue(
	ctx context.Context,
	tenantID uint64,
	facts types.EnterpriseAdministrationFacts,
) (*types.EnterpriseAdministrationPlatformProjection, error) {
	var projection types.EnterpriseAdministrationPlatformProjection
	if err := c.post(ctx, tenantID, "enterprise-administration-queue", facts, &projection); err != nil ||
		projection.ContractVersion != types.EnterpriseAdministrationQueueV1 ||
		projection.Scope.Kind != "enterprise_assigned" ||
		projection.Scope.ProductBaseTenantID != strconv.FormatUint(tenantID, 10) ||
		strings.TrimSpace(projection.AsOf) == "" {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	for _, item := range projection.Items {
		if !validEnterpriseAdministrationItem(item) {
			return nil, interfaces.ErrAICapabilityUnavailable
		}
	}
	return &projection, nil
}

func validEnterpriseAdministrationItem(item types.EnterpriseAdministrationItem) bool {
	validCode := item.Code == "license_or_capability_attention" ||
		item.Code == "edge_node_offline" ||
		item.Code == "operating_analysis_access_gap"
	validPriority := item.Priority == "critical" || item.Priority == "high" || item.Priority == "medium"
	validTarget := item.Target == "members" || item.Target == "service_health"
	return validCode && validPriority && validTarget && item.Count > 0
}

func (c *Client) get(ctx context.Context, tenantID uint64, route string, target any) error {
	return c.request(ctx, http.MethodGet, tenantID, route, nil, target)
}

func (c *Client) post(ctx context.Context, tenantID uint64, route string, body any, target any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return interfaces.ErrAICapabilityUnavailable
	}
	return c.request(ctx, http.MethodPost, tenantID, route, payload, target)
}

func (c *Client) request(
	ctx context.Context,
	method string,
	tenantID uint64,
	route string,
	body []byte,
	target any,
) error {
	if tenantID == 0 || c.baseURL == "" || c.token == "" {
		return interfaces.ErrAICapabilityUnavailable
	}
	endpoint := fmt.Sprintf(
		"%s/api/internal/product-base/tenants/%s/%s",
		c.baseURL,
		url.PathEscape(strconv.FormatUint(tenantID, 10)),
		route,
	)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return interfaces.ErrAICapabilityUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return interfaces.ErrAICapabilityUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return interfaces.ErrAICapabilityUnavailable
	}
	decoder := json.NewDecoder(res.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return interfaces.ErrAICapabilityUnavailable
	}
	return nil
}
