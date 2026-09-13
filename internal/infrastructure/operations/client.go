package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const maxOperationsResponseBytes = 2 << 20

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClientFromEnv() interfaces.PlatformOperationsBridge {
	return &Client{baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("RINGXUN_CAPABILITY_PLAN_BASE_URL")), "/"),
		token: strings.TrimSpace(os.Getenv("RINGXUN_CAPABILITY_PLAN_SERVICE_TOKEN")),
		http:  &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) getJSON(ctx context.Context, method, path, actorUserID string, output any) error {
	if c.baseURL == "" || c.token == "" || actorUserID == "" || !strings.HasPrefix(path, "/api/internal/product-base/operations/") {
		return fmt.Errorf("platform operations bridge is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Product-Base-Actor-User-Id", actorUserID)
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Center operations unavailable")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxOperationsResponseBytes+1))
	if err != nil || len(data) > maxOperationsResponseBytes {
		return fmt.Errorf("invalid Center operations response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var body struct {
			Detail json.RawMessage `json:"detail"`
		}
		_ = json.Unmarshal(data, &body)
		var detail string
		if json.Unmarshal(body.Detail, &detail) != nil {
			var pending struct {
				Code            string `json:"code"`
				NodeStatus      string `json:"node_status"`
				ControlRevision int64  `json:"control_revision"`
			}
			if json.Unmarshal(body.Detail, &pending) == nil && response.StatusCode == http.StatusServiceUnavailable && pending.Code == "edge_node_disabled_projection_pending" && pending.NodeStatus == "disabled" && pending.ControlRevision > 0 {
				detail = pending.Code
			}
		}
		return &interfaces.PlatformOperationsUpstreamError{StatusCode: response.StatusCode, Detail: detail}
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("invalid Center operations response")
	}
	return nil
}

func (c *Client) GetEnterpriseEdge(ctx context.Context, enterpriseID, actorUserID string) (*interfaces.CenterEnterpriseEdge, error) {
	var output interfaces.CenterEnterpriseEdge
	err := c.getJSON(ctx, http.MethodGet, "/api/internal/product-base/operations/enterprises/"+url.PathEscape(enterpriseID)+"/edge", actorUserID, &output)
	if err != nil {
		return nil, err
	}
	if output.Schema != "CenterEnterpriseEdgeV1" || output.EnterpriseID != enterpriseID {
		return nil, fmt.Errorf("invalid Center Edge response")
	}
	return &output, nil
}

func (c *Client) RotateEnterpriseEnrollmentToken(ctx context.Context, enterpriseID, actorUserID string) (*interfaces.CenterEnterpriseEnrollment, error) {
	var output interfaces.CenterEnterpriseEnrollment
	err := c.getJSON(ctx, http.MethodPost, "/api/internal/product-base/operations/enterprises/"+url.PathEscape(enterpriseID)+"/edge-enrollment-token/rotate", actorUserID, &output)
	if err != nil {
		return nil, err
	}
	if output.Schema != "CenterEnterpriseEdgeEnrollmentV1" || output.EnterpriseID != enterpriseID || output.EnrollmentToken == "" {
		return nil, fmt.Errorf("invalid Center enrollment response")
	}
	return &output, nil
}

func (c *Client) DisableEnterpriseEdgeNode(ctx context.Context, enterpriseID, edgeNodeID, actorUserID string) (*interfaces.CenterEdgeNodeDisable, error) {
	var output interfaces.CenterEdgeNodeDisable
	path := "/api/internal/product-base/operations/enterprises/" + url.PathEscape(enterpriseID) + "/edge/nodes/" + url.PathEscape(edgeNodeID) + "/disable"
	if err := c.getJSON(ctx, http.MethodPost, path, actorUserID, &output); err != nil {
		return nil, err
	}
	if output.Schema != "CenterEdgeNodeDisableV1" || output.EnterpriseID != enterpriseID || output.EdgeNodeID != edgeNodeID || output.Status != "disabled" || output.ControlRevision < 1 {
		return nil, fmt.Errorf("invalid Center node disable response")
	}
	return &output, nil
}
