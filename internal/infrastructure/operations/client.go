package operations

import (
	"context"
	"fmt"
	"io"
	"net/http"
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
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("RINGXUN_CAPABILITY_PLAN_BASE_URL")), "/"),
		token:   strings.TrimSpace(os.Getenv("RINGXUN_CAPABILITY_PLAN_SERVICE_TOKEN")),
		http: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *Client) Do(ctx context.Context, method, path, actorUserID, idempotencyKey string, body io.Reader) (*interfaces.PlatformOperationsResponse, error) {
	if c.baseURL == "" || c.token == "" {
		return nil, fmt.Errorf("platform operations bridge is not configured")
	}
	if actorUserID == "" || !strings.HasPrefix(path, "/api/internal/product-base/operations/") {
		return nil, fmt.Errorf("invalid platform operations request")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Product-Base-Actor-User-Id", actorUserID)
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("platform operations service unavailable")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxOperationsResponseBytes+1))
	if err != nil || len(data) > maxOperationsResponseBytes {
		return nil, fmt.Errorf("invalid platform operations response")
	}
	return &interfaces.PlatformOperationsResponse{
		StatusCode:  response.StatusCode,
		ContentType: response.Header.Get("Content-Type"),
		Body:        data,
	}, nil
}
