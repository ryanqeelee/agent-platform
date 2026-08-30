package capabilityplan

import (
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

func (c *Client) get(ctx context.Context, tenantID uint64, route string, target any) error {
	if tenantID == 0 || c.baseURL == "" || c.token == "" {
		return interfaces.ErrAICapabilityUnavailable
	}
	endpoint := fmt.Sprintf(
		"%s/api/internal/product-base/tenants/%s/%s",
		c.baseURL,
		url.PathEscape(strconv.FormatUint(tenantID, 10)),
		route,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return interfaces.ErrAICapabilityUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
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
