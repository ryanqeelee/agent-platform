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
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("RINGXUN_CAPABILITY_PLAN_BASE_URL")), "/"),
		token:   strings.TrimSpace(os.Getenv("RINGXUN_CAPABILITY_PLAN_SERVICE_TOKEN")),
		http:    &http.Client{Timeout: 3 * time.Second},
	}
}

func (c *Client) Resolve(ctx context.Context, tenantID uint64) (*types.AICapabilityPlanResolution, error) {
	if tenantID == 0 || c.baseURL == "" || c.token == "" {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	endpoint := fmt.Sprintf(
		"%s/api/internal/product-base/tenants/%s/ai-capability-plan",
		c.baseURL,
		url.PathEscape(strconv.FormatUint(tenantID, 10)),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, interfaces.ErrAICapabilityUnavailable
	}
	var resolution types.AICapabilityPlanResolution
	decoder := json.NewDecoder(res.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&resolution); err != nil ||
		resolution.ContractVersion != "AICapabilityPlanV1" ||
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
