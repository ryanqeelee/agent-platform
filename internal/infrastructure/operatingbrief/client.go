package operatingbrief

import (
	"bytes"
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

const maxComputeResponse = 8 * 1024 * 1024

type client struct {
	endpoint string
	token    string
	http     *http.Client
}

func NewClient() interfaces.OperatingBriefComputeClient {
	return &client{
		endpoint: strings.TrimSpace(os.Getenv("RINGXUN_OPERATING_BRIEF_COMPUTE_URL")),
		token:    strings.TrimSpace(os.Getenv("RINGXUN_OPERATING_BRIEF_COMPUTE_TOKEN")),
		http: &http.Client{
			Timeout:       2 * time.Minute,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *client) Compute(ctx context.Context, payload map[string]any) (map[string]any, error) {
	if c.endpoint == "" || c.token == "" {
		return nil, fmt.Errorf("operating brief compute is not configured")
	}
	parsed, err := url.Parse(c.endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid operating brief compute URL")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("operating brief compute request failed")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxComputeResponse+1))
	if err != nil || len(data) > maxComputeResponse {
		return nil, fmt.Errorf("operating brief compute response is invalid")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("operating brief compute rejected input")
	}
	result := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("operating brief compute response is invalid")
	}
	return result, nil
}
