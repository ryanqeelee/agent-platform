package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
)

func TestSystemAdminInfoAndDocReaderRoutesUseBrowserOnlyAdminGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	guards := &rbacGuards{apiKeyAuthorizer: middleware.NewAPIKeyRouteAuthorizer()}
	router := gin.New()
	router.Use(testPlatformPrincipalContext())
	router.Use(guards.apiKeyAuthorizer.Middleware())
	RegisterSystemRoutes(router.Group("/api/v1"), handler.NewSystemHandler(
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
	), guards)

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
		{name: "admin reconnect reaches validation", method: http.MethodPost, path: "/api/v1/system/admin/docreader/reconnect", body: `{}`, admin: true, wantStatus: http.StatusBadRequest},
		{name: "ordinary user reconnect", method: http.MethodPost, path: "/api/v1/system/admin/docreader/reconnect", body: `{}`, wantStatus: http.StatusForbidden},
		{name: "API key reconnect", method: http.MethodPost, path: "/api/v1/system/admin/docreader/reconnect", body: `{}`, apiKey: true, wantStatus: http.StatusForbidden},
		{name: "old reconnect removed", method: http.MethodPost, path: "/api/v1/system/docreader/reconnect", body: `{}`, admin: true, wantStatus: http.StatusNotFound},
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
		{http.MethodPost, "/api/v1/system/admin/docreader/reconnect"},
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
			ctx = context.WithValue(ctx, types.UserContextKey, &types.User{
				ID: "caller", IsActive: true, IsSystemAdmin: admin,
			})
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
