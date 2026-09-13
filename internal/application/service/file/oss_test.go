package file

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/utils"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss"
	"github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss/credentials"
)

func TestParseOssFilePath(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantBucket  string
		wantKey     string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid path with nested key",
			input:      "oss://my-bucket/123/exports/abc123.csv",
			wantBucket: "my-bucket",
			wantKey:    "123/exports/abc123.csv",
		},
		{
			name:       "valid path with simple key",
			input:      "oss://test-bucket/key",
			wantBucket: "test-bucket",
			wantKey:    "key",
		},
		{
			name:       "valid path with deep nesting",
			input:      "oss://bucket/prefix/tenant/exports/uuid.png",
			wantBucket: "bucket",
			wantKey:    "prefix/tenant/exports/uuid.png",
		},
		{
			name:        "invalid scheme",
			input:       "s3://bucket/key",
			wantErr:     true,
			errContains: "invalid OSS file path",
		},
		{
			name:        "empty path",
			input:       "",
			wantErr:     true,
			errContains: "invalid OSS file path",
		},
		{
			name:        "bucket only no key",
			input:       "oss://bucket/",
			wantErr:     true,
			errContains: "invalid OSS file path",
		},
		{
			name:        "scheme only",
			input:       "oss://",
			wantErr:     true,
			errContains: "invalid OSS file path",
		},
		{
			name:        "no slash after bucket",
			input:       "oss://bucket",
			wantErr:     true,
			errContains: "invalid OSS file path",
		},
		{
			name:        "empty bucket name",
			input:       "oss:///some-key",
			wantErr:     true,
			errContains: "invalid OSS file path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket, key, err := parseOssFilePath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseOssFilePath(%q) expected error, got bucket=%q key=%q", tt.input, bucket, key)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("parseOssFilePath(%q) error = %v, want containing %q", tt.input, err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Errorf("parseOssFilePath(%q) unexpected error: %v", tt.input, err)
				return
			}
			if bucket != tt.wantBucket {
				t.Errorf("parseOssFilePath(%q) bucket = %q, want %q", tt.input, bucket, tt.wantBucket)
			}
			if key != tt.wantKey {
				t.Errorf("parseOssFilePath(%q) key = %q, want %q", tt.input, key, tt.wantKey)
			}
		})
	}
}

func TestNewOSSClient(t *testing.T) {
	const endpoint = "https://oss-unit.test"
	t.Setenv("SSRF_WHITELIST", "oss-unit.test")
	utils.ResetSSRFWhitelistForTest()
	t.Cleanup(utils.ResetSSRFWhitelistForTest)

	client, err := newOSSClient(endpoint, "cn-hangzhou", "test-access-key", "test-secret-key")
	if err != nil {
		t.Fatalf("newOSSClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("newOSSClient() returned a nil client")
	}
}

func TestNewOSSClientRejectsUnsafeEndpoint(t *testing.T) {
	if _, err := newOSSClient("http://127.0.0.1:9000", "cn-hangzhou", "ak", "sk"); err == nil {
		t.Fatal("expected loopback OSS endpoint to be rejected")
	}
}

func TestCheckOssConnectivity_RejectsUnsafeEndpoint(t *testing.T) {
	err := CheckOssConnectivity(context.Background(),
		"http://127.0.0.1:9000",
		"cn-hangzhou",
		"access-key",
		"secret-key",
		"bucket",
	)
	if err == nil {
		t.Fatal("CheckOssConnectivity() expected an unsafe endpoint error")
	}
	if !strings.Contains(err.Error(), "unsafe OSS endpoint") {
		t.Fatalf("CheckOssConnectivity() error = %v, want unsafe endpoint error", err)
	}
}

func TestOssEnsureBucket_CheckFails(t *testing.T) {
	transportErr := errors.New("mock bucket check failure")
	client := newMockOSSClient(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})

	err := ossEnsureBucket(client, "test-bucket")
	if err == nil {
		t.Fatal("ossEnsureBucket() expected a bucket check error")
	}
	if !strings.Contains(err.Error(), "failed to check OSS bucket") || !errors.Is(err, transportErr) {
		t.Fatalf("ossEnsureBucket() error = %v, want wrapped bucket check error", err)
	}
}

func TestOssEnsureBucket_CreateFails(t *testing.T) {
	createErr := errors.New("mock bucket creation failure")
	client := newMockOSSClient(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodGet {
			return mockOSSResponse(req, http.StatusNotFound, `<?xml version="1.0" encoding="UTF-8"?>
<Error><Code>NoSuchBucket</Code><Message>not found</Message><RequestId>test-request</RequestId></Error>`), nil
		}
		return nil, createErr
	})

	err := ossEnsureBucket(client, "test-bucket")
	if err == nil {
		t.Fatal("ossEnsureBucket() expected a bucket creation error")
	}
	if !strings.Contains(err.Error(), "failed to create OSS bucket") || !errors.Is(err, createErr) {
		t.Fatalf("ossEnsureBucket() error = %v, want wrapped bucket creation error", err)
	}
}

type ossRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn ossRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newMockOSSClient(roundTrip ossRoundTripFunc) *oss.Client {
	cfg := oss.LoadDefaultConfig().
		WithCredentialsProvider(credentials.NewStaticCredentialsProvider("access-key", "secret-key", "")).
		WithRegion("cn-hangzhou").
		WithEndpoint("https://oss-unit.test").
		WithRetryMaxAttempts(1).
		WithHttpClient(&http.Client{Transport: roundTrip})
	return oss.NewClient(cfg)
}

func mockOSSResponse(req *http.Request, statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header: http.Header{
			"Content-Type":     []string{"application/xml"},
			"X-Oss-Request-Id": []string{"test-request"},
		},
		Body:    io.NopCloser(strings.NewReader(body)),
		Request: req,
	}
}
