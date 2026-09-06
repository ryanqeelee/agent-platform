package web_fetch

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestFetcherFetchSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("<html><body><main>official specifications</main></body></html>"))
	}))
	defer server.Close()

	fetcher := newTestFetcher(server.Client())
	content, err := fetcher.Fetch(context.Background(), server.URL)

	require.NoError(t, err)
	assert.Contains(t, content, "official specifications")
}

func TestFetcherClassifiesHTTPStatus(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		code      ErrorCode
		retryable bool
	}{
		{name: "forbidden", status: http.StatusForbidden, code: ErrorHTTP403, retryable: false},
		{name: "rate limited", status: http.StatusTooManyRequests, code: ErrorHTTP429, retryable: true},
		{name: "server error", status: http.StatusServiceUnavailable, code: ErrorHTTP5xx, retryable: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
			}))
			defer server.Close()

			_, err := newTestFetcher(server.Client()).Fetch(context.Background(), server.URL)
			code, retryable, _ := ErrorDetails(err)
			assert.Equal(t, test.code, code)
			assert.Equal(t, test.retryable, retryable)
		})
	}
}

func TestFetcherClassifiesNetworkFailures(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		code      ErrorCode
		retryable bool
	}{
		{name: "dns", err: &net.DNSError{Err: "no such host", Name: "invalid.example"}, code: ErrorDNS, retryable: true},
		{name: "timeout", err: context.DeadlineExceeded, code: ErrorTimeout, retryable: true},
		{name: "tls", err: x509.HostnameError{Host: "example.com"}, code: ErrorTLS, retryable: false},
		{name: "redirect", err: errors.New("redirect blocked by SSRF private address"), code: ErrorRedirectRejected, retryable: false},
		{name: "dial-time SSRF", err: errors.New("connection blocked: host resolves to restricted IP"), code: ErrorSSRFRejected, retryable: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, test.err
			})}
			_, err := newTestFetcher(client).Fetch(context.Background(), "https://example.com")
			code, retryable, _ := ErrorDetails(err)
			assert.Equal(t, test.code, code)
			assert.Equal(t, test.retryable, retryable)
		})
	}
}

func TestFetcherClassifiesDNSFailureDuringSSRFValidation(t *testing.T) {
	fetcher := newTestFetcher(&http.Client{})
	fetcher.validateURL = func(string) error {
		return errors.New("SSRF validation failed: DNS resolution failed for hostname unavailable.example")
	}

	_, err := fetcher.Fetch(context.Background(), "https://unavailable.example")
	code, retryable, _ := ErrorDetails(err)

	assert.Equal(t, ErrorDNS, code)
	assert.True(t, retryable)
}

func TestFetcherRejectsEmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("<html><body><script>ignored()</script></body></html>"))
	}))
	defer server.Close()

	_, err := newTestFetcher(server.Client()).Fetch(context.Background(), server.URL)
	code, retryable, _ := ErrorDetails(err)
	assert.Equal(t, ErrorEmptyContent, code)
	assert.False(t, retryable)
}

func TestFetcherUsesBrowserFallbackForClientRenderedPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`<html><body><div id="app">Loading...</div><script>render()</script></body></html>`))
	}))
	defer server.Close()

	fetcher := newTestFetcher(server.Client())
	fetcher.resolveIPs = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	fetcher.renderBrowser = func(context.Context, pinnedTarget) (string, error) {
		return `<html><body><main>rendered product specifications</main></body></html>`, nil
	}

	content, err := fetcher.Fetch(context.Background(), server.URL)

	require.NoError(t, err)
	assert.Contains(t, content, "rendered product specifications")
}

func TestNewFetcherKeepsAgentCompatibleTimeout(t *testing.T) {
	assert.Equal(t, 60*time.Second, NewFetcher().timeout)
}

func TestNewPipelineFetcherUsesHTTPOnlyAndLegacyTimeout(t *testing.T) {
	fetcher := NewPipelineFetcher()
	assert.Equal(t, 15*time.Second, fetcher.timeout)
	assert.Nil(t, fetcher.renderBrowser)
}

func TestFetcherReturnsErrorWhenBrowserFallbackFailsOnSPA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`<html><body><div id="app">Loading...</div><script>render()</script></body></html>`))
	}))
	defer server.Close()

	fetcher := newTestFetcher(server.Client())
	fetcher.resolveIPs = func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	fetcher.renderBrowser = func(context.Context, pinnedTarget) (string, error) {
		return "", errors.New("browser unavailable")
	}

	_, err := fetcher.Fetch(context.Background(), server.URL)
	code, retryable, _ := ErrorDetails(err)
	assert.Equal(t, ErrorEmptyContent, code)
	assert.False(t, retryable)
}

func TestFetcherClassifiesInvalidAndSSRFURLs(t *testing.T) {
	fetcher := NewFetcher()
	_, invalidErr := fetcher.Fetch(context.Background(), "not-a-url")
	invalidCode, invalidRetryable, _ := ErrorDetails(invalidErr)
	assert.Equal(t, ErrorInvalidURL, invalidCode)
	assert.False(t, invalidRetryable)

	_, ssrfErr := fetcher.Fetch(context.Background(), "http://127.0.0.1:1/private")
	ssrfCode, ssrfRetryable, _ := ErrorDetails(ssrfErr)
	assert.Equal(t, ErrorSSRFRejected, ssrfCode)
	assert.False(t, ssrfRetryable)
}

func TestPinnedDialUsesValidatedIPAndPreservesPort(t *testing.T) {
	var dialedAddress string
	fetcher := &Fetcher{
		resolveIPs: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		},
		dialContext: func(_ context.Context, _, address string) (net.Conn, error) {
			dialedAddress = address
			return nil, errors.New("stop dial")
		},
	}

	_, err := fetcher.pinnedDialContext()(context.Background(), "tcp", "example.com:443")

	assert.Equal(t, "93.184.216.34:443", dialedAddress)
	assert.EqualError(t, err, "stop dial")
}

func TestPinnedDialRejectsRebindingToRestrictedIP(t *testing.T) {
	dialCalled := false
	fetcher := &Fetcher{
		resolveIPs: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("93.184.216.34"), net.ParseIP("127.0.0.1")}, nil
		},
		dialContext: func(context.Context, string, string) (net.Conn, error) {
			dialCalled = true
			return nil, nil
		},
	}

	_, err := fetcher.pinnedDialContext()(context.Background(), "tcp", "example.com:443")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection blocked")
	assert.False(t, dialCalled)
}

func TestFetcherKeepsOriginalHostForTLSAndHTTPRouting(t *testing.T) {
	var requestURL string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestURL = request.URL.Host
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       http.NoBody,
			Request:    request,
		}, nil
	})}
	fetcher := newTestFetcher(client)
	fetcher.validateURL = func(string) error { return nil }

	_, err := fetcher.Fetch(context.Background(), "https://example.com/specs")

	require.Error(t, err)
	assert.Equal(t, "example.com", requestURL)
	assert.Contains(t, err.Error(), "no readable text")
}

func newTestFetcher(client *http.Client) *Fetcher {
	return &Fetcher{
		client:      client,
		timeout:     time.Second,
		maxBodySize: maxBodySize,
		validateURL: func(string) error { return nil },
	}
}

func TestFetchPreservesBodyAfterLargeHead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><head><script>" + strings.Repeat(" ", 150*1024) + "</script></head><body><main>门店退货流程：请保留购买凭证。</main></body></html>"))
	}))
	defer server.Close()
	text, err := newTestFetcher(server.Client()).Fetch(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Contains(t, text, "请保留购买凭证")
}

func TestFetchRejectsOversizeInsteadOfReturningPartialEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat("a", 1025))) }))
	defer server.Close()
	f := newTestFetcher(server.Client())
	f.maxBodySize = 1024
	_, err := f.Fetch(context.Background(), server.URL)
	code, retryable, _ := ErrorDetails(err)
	assert.Equal(t, ErrorBodyTooLarge, code)
	assert.False(t, retryable)
}

func TestExtractArticleExcludesNavigation(t *testing.T) {
	paragraph := strings.Repeat("门店交接班需要核对商品、库存和收银记录，异常情况应记录并移交。", 20)
	source := "<html><body><nav>导航噪声</nav><article><h1>门店交接班</h1><p>" + paragraph + "</p></article><aside>推荐广告</aside></body></html>"
	text, err := extractPageText(source, "https://example.com/article")
	require.NoError(t, err)
	assert.Contains(t, text, "异常情况应记录并移交")
	assert.NotContains(t, text, "推荐广告")
	assert.NotContains(t, text, "导航噪声")
}

func TestFetchPreservesMarkdownWithoutArticleExtraction(t *testing.T) {
	markdown := "# 产品说明\n" + strings.Repeat("完整的产品使用说明与功能介绍。\n", 100) + "<details><summary>示例</summary>必须保留</details>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(markdown))
	}))
	defer server.Close()
	text, err := newTestFetcher(server.Client()).Fetch(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, markdown, text)
}

func TestFetchPreservesTextBoundaryWhitespace(t *testing.T) {
	for _, contentType := range []string{"text/plain", "text/markdown"} {
		t.Run(contentType, func(t *testing.T) {
			body := "    indented code block\n\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", contentType)
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			content, err := newTestFetcher(server.Client()).Fetch(context.Background(), server.URL)
			require.NoError(t, err)
			assert.Equal(t, body, content)
		})
	}
}
