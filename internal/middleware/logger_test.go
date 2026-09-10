package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	appLogger "github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func TestSanitizeBody(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "camelCase apiKey",
			in:   `{"modelName":"gpt-5.2","apiKey":"sk-secret-123","provider":"azure_openai"}`,
			want: `{"modelName":"gpt-5.2","apiKey":"***","provider":"azure_openai"}`,
		},
		{
			name: "snake_case api_key",
			in:   `{"api_key":"sk-secret-123"}`,
			want: `{"api_key":"***"}`,
		},
		{
			name: "PascalCase APIKey",
			in:   `{"APIKey":"sk-secret-123"}`,
			want: `{"APIKey":"***"}`,
		},
		{
			name: "secretKey camelCase",
			in:   `{"secretKey":"abc","accessKeyId":"id"}`,
			want: `{"secretKey":"***","accessKeyId":"id"}`,
		},
		{
			name: "refreshToken / accessToken camelCase",
			in:   `{"refreshToken":"rt","accessToken":"at"}`,
			want: `{"refreshToken":"***","accessToken":"***"}`,
		},
		{
			name: "password and token preserved as masked",
			in:   `{"password":"p","token":"t"}`,
			want: `{"password":"***","token":"***"}`,
		},
		{
			name: "snake_case new_password and old_password",
			in:   `{"email":"alice@example.com","new_password":"FreshPass9","old_password":"OldPass9"}`,
			want: `{"email":"alice@example.com","new_password":"***","old_password":"***"}`,
		},
		{
			name: "extra whitespace around colon",
			in:   `{"apiKey"  :   "leak"}`,
			want: `{"apiKey":"***"}`,
		},
		{
			name: "non sensitive fields untouched",
			in:   `{"baseUrl":"https://example.com","modelName":"gpt"}`,
			want: `{"baseUrl":"https://example.com","modelName":"gpt"}`,
		},
		{
			name: "OAuth authorization response fields",
			in:   `{"authorization_url":"https://idp.example/authorize?state=secret","authorization_attempt":"secret-state"}`,
			want: `{"authorization_url":"***","authorization_attempt":"***"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeBody(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeBody(%q)\n got: %s\nwant: %s", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeQuery(t *testing.T) {
	got := sanitizeQuery("code=secret-code&state=secret-state&next=%2Fsettings&state=second")
	want := "code=%2A%2A%2A&next=%2Fsettings&state=%2A%2A%2A"
	if got != want {
		t.Fatalf("sanitizeQuery() = %q, want %q", got, want)
	}
}

func TestLoggerKeepsGovernedNativeQAMetadataWithoutBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	appLogger.SetOutput(&logs)
	appLogger.SetLogLevel(appLogger.LevelInfo)
	t.Cleanup(func() {
		appLogger.SetOutput(os.Stdout)
		appLogger.SetLogLevel(appLogger.LevelDebug)
	})

	router := gin.New()
	router.Use(Logger())
	router.POST("/api/v1/agent-chat/:session_id", func(c *gin.Context) {
		c.Request = c.Request.WithContext(types.WithGovernedDataObservability(c.Request.Context()))
		c.JSON(http.StatusOK, gin.H{"answer": "ROWS_SENTINEL"})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-chat/session-1", strings.NewReader(`{"query":"QUERY_SENTINEL"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	got := logs.String()
	for _, secret := range []string{"QUERY_SENTINEL", "ROWS_SENTINEL", "request_body", "response_body"} {
		if strings.Contains(got, secret) {
			t.Fatalf("native QA access log leaked %q: %s", secret, got)
		}
	}
	for _, metadata := range []string{"method=POST", "path=/api/v1/agent-chat/session-1", "status_code=200", "size="} {
		if !strings.Contains(got, metadata) {
			t.Fatalf("native QA access log lost %q: %s", metadata, got)
		}
	}
}

func TestLoggerNativeQAErrorKeepsMetadataWithoutBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	appLogger.SetOutput(&logs)
	appLogger.SetLogLevel(appLogger.LevelInfo)
	t.Cleanup(func() {
		appLogger.SetOutput(os.Stdout)
		appLogger.SetLogLevel(appLogger.LevelDebug)
	})

	router := gin.New()
	router.Use(Logger())
	router.POST("/api/v1/agent-chat/:session_id", func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "ROWS_SENTINEL"})
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-chat/session-1", strings.NewReader(`{"query":"QUERY_SENTINEL"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), req)

	got := logs.String()
	for _, secret := range []string{"QUERY_SENTINEL", "ROWS_SENTINEL", "request_body", "response_body"} {
		if strings.Contains(got, secret) {
			t.Fatalf("failed native QA access log leaked %q: %s", secret, got)
		}
	}
	for _, metadata := range []string{"method=POST", "status_code=400", "size="} {
		if !strings.Contains(got, metadata) {
			t.Fatalf("failed native QA access log lost %q: %s", metadata, got)
		}
	}
}

func TestLoggerOrdinaryNativeQAStillRecordsRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	appLogger.SetOutput(&logs)
	appLogger.SetLogLevel(appLogger.LevelInfo)
	t.Cleanup(func() {
		appLogger.SetOutput(os.Stdout)
		appLogger.SetLogLevel(appLogger.LevelDebug)
	})

	router := gin.New()
	router.Use(Logger())
	router.POST("/api/v1/agent-chat/:session_id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"answer": "ordinary-response"})
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-chat/session-1", strings.NewReader(`{"query":"ordinary-query"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), req)

	got := logs.String()
	for _, want := range []string{"ordinary-query", "ordinary-response", "request_body", "response_body", "method=POST", "status_code=200"} {
		if !strings.Contains(got, want) {
			t.Fatalf("ordinary native QA access log lost %q: %s", want, got)
		}
	}
}

func TestLoggerOrdinaryRouteStillRecordsBodies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	appLogger.SetOutput(&logs)
	appLogger.SetLogLevel(appLogger.LevelInfo)
	t.Cleanup(func() {
		appLogger.SetOutput(os.Stdout)
		appLogger.SetLogLevel(appLogger.LevelDebug)
	})

	router := gin.New()
	router.Use(Logger())
	router.POST("/api/v1/ordinary", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"answer": "ordinary-response"})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ordinary", strings.NewReader(`{"query":"ordinary-query"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), req)

	got := logs.String()
	for _, want := range []string{"ordinary-query", "ordinary-response", "request_body", "response_body"} {
		if !strings.Contains(got, want) {
			t.Fatalf("ordinary access log lost %q: %s", want, got)
		}
	}
}
