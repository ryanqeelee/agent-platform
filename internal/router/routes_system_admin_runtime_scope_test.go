package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

type platformParserConfigRepoStub struct {
	row *types.PlatformParserConfig
}

func (r *platformParserConfigRepoStub) Get(context.Context) (*types.PlatformParserConfig, error) {
	return r.row, nil
}

func (r *platformParserConfigRepoStub) Upsert(
	_ context.Context,
	config *types.ParserEngineConfig,
	actorID string,
	now time.Time,
) (*types.PlatformParserConfig, error) {
	r.row = &types.PlatformParserConfig{
		ID: types.PlatformParserConfigSingletonID, Config: config,
		UpdatedBy: actorID, CreatedAt: now, UpdatedAt: now,
	}
	return r.row, nil
}

func TestSystemAdminParserRoutesUseBrowserOnlyAdminGuard(t *testing.T) {
	t.Setenv("SYSTEM_AES_KEY", strings.Repeat("k", 32))
	gin.SetMode(gin.TestMode)
	guards := &rbacGuards{apiKeyAuthorizer: middleware.NewAPIKeyRouteAuthorizer()}
	repo := &platformParserConfigRepoStub{row: &types.PlatformParserConfig{
		ID: types.PlatformParserConfigSingletonID,
		Config: &types.ParserEngineConfig{
			MinerUEndpoint: "https://private-parser.example.invalid",
		},
	}}
	parserConfig := service.NewPlatformParserConfigService(repo)
	systemHandler := handler.NewSystemHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, parserConfig,
	)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.Use(testPlatformPrincipalContext())
	router.Use(guards.apiKeyAuthorizer.Middleware())
	v1 := router.Group("/api/v1")
	RegisterSystemRoutes(v1, systemHandler, guards)
	RegisterSystemAdminRoutes(v1, systemHandler, nil, nil, guards)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		admin      bool
		apiKey     bool
		wantStatus int
		wantBody   string
		notBody    string
	}{
		{name: "admin info", method: http.MethodGet, path: "/api/v1/system/admin/info", admin: true, wantStatus: http.StatusOK},
		{name: "ordinary user info", method: http.MethodGet, path: "/api/v1/system/admin/info", wantStatus: http.StatusForbidden},
		{name: "API key info", method: http.MethodGet, path: "/api/v1/system/admin/info", apiKey: true, wantStatus: http.StatusForbidden},
		{name: "enterprise info retained", method: http.MethodGet, path: "/api/v1/system/info", wantStatus: http.StatusOK, wantBody: `"edition"`, notBody: `"version"`},
		{name: "admin parser config", method: http.MethodGet, path: "/api/v1/system/admin/parser-engine-config", admin: true, wantStatus: http.StatusOK},
		{name: "ordinary user parser config", method: http.MethodGet, path: "/api/v1/system/admin/parser-engine-config", wantStatus: http.StatusForbidden},
		{name: "API key parser config", method: http.MethodGet, path: "/api/v1/system/admin/parser-engine-config", apiKey: true, wantStatus: http.StatusForbidden},
		{name: "admin parser engines", method: http.MethodGet, path: "/api/v1/system/admin/parser-engines", admin: true, wantStatus: http.StatusOK, notBody: "weknoracloud"},
		{name: "admin parser check", method: http.MethodPost, path: "/api/v1/system/admin/parser-engines/check", body: `{}`, admin: true, wantStatus: http.StatusOK, notBody: "weknoracloud"},
		{name: "tenant parser capability catalog", method: http.MethodGet, path: "/api/v1/system/parser-engines", wantStatus: http.StatusOK, wantBody: `"Name"`, notBody: "docreader_addr"},
		{name: "admin parser update masks secret", method: http.MethodPut, path: "/api/v1/system/admin/parser-engine-config", body: `{"mineru_api_key":"new-secret"}`, admin: true, wantStatus: http.StatusOK, wantBody: `"mineru_api_key":"***"`, notBody: "new-secret"},
		{name: "ordinary user parser check", method: http.MethodPost, path: "/api/v1/system/admin/parser-engines/check", body: `{}`, wantStatus: http.StatusForbidden},
		{name: "API key parser check", method: http.MethodPost, path: "/api/v1/system/admin/parser-engines/check", body: `{}`, apiKey: true, wantStatus: http.StatusForbidden},
		{name: "old reconnect removed", method: http.MethodPost, path: "/api/v1/system/docreader/reconnect", body: `{}`, admin: true, wantStatus: http.StatusNotFound},
	}

	capabilitiesRequest := httptest.NewRequest(http.MethodGet, "/api/v1/system/parser-engines", nil)
	capabilitiesResponse := httptest.NewRecorder()
	router.ServeHTTP(capabilitiesResponse, capabilitiesRequest)
	for _, forbidden := range []string{
		"private-parser.example.invalid", "stored-secret", "new-secret",
		"mineru_endpoint", "mineru_api_key", "docreader_addr", "docreader_transport", "connected", "UnavailableReason", "weknoracloud",
	} {
		if strings.Contains(capabilitiesResponse.Body.String(), forbidden) {
			t.Fatalf("tenant parser capability catalog leaked %q: %s", forbidden, capabilitiesResponse.Body.String())
		}
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			if test.admin {
				request.Header.Set("X-Test-System-Admin", "true")
			}
			if test.apiKey {
				request.Header.Set("X-Test-API-Key", "true")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			if test.wantBody != "" && !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body = %s, want it to contain %s", response.Body.String(), test.wantBody)
			}
			if test.notBody != "" && strings.Contains(response.Body.String(), test.notBody) {
				t.Fatalf("body = %s, want it not to contain %s", response.Body.String(), test.notBody)
			}
		})
	}

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/system/admin/info"},
		{http.MethodGet, "/api/v1/system/admin/parser-engines"},
		{http.MethodPost, "/api/v1/system/admin/parser-engines/check"},
		{http.MethodGet, "/api/v1/system/admin/parser-engine-config"},
		{http.MethodPut, "/api/v1/system/admin/parser-engine-config"},
	} {
		if _, ok := guards.apiKeyAuthorizer.Lookup(route.method, route.path); ok {
			t.Fatalf("API key policy unexpectedly declared for %s %s", route.method, route.path)
		}
	}
}

func testPlatformPrincipalContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if c.GetHeader("X-Test-API-Key") == "true" {
			ctx = types.WithTenantAPIKeyScope(ctx, types.TenantAPIKeyScope{
				ScopeType:  types.APIKeyScopePlatform,
				FullAccess: true,
			})
		} else {
			admin := c.GetHeader("X-Test-System-Admin") == "true"
			ctx = context.WithValue(ctx, types.SystemAdminContextKey, admin)
			ctx = context.WithValue(ctx, types.UserIDContextKey, "caller")
			ctx = context.WithValue(ctx, types.UserContextKey, &types.User{
				ID: "caller", IsActive: true, IsSystemAdmin: admin,
			})
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
