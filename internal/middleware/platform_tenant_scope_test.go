package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type platformTenantScopeService struct {
	interfaces.TenantService
	tenant *types.Tenant
	calls  int
}

func (s *platformTenantScopeService) GetTenantByID(context.Context, uint64) (*types.Tenant, error) {
	s.calls++
	return s.tenant, nil
}

func TestBindSystemAdminTenantScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []string{
		types.TenantStatusActive,
		types.TenantStatusSuspended,
		types.TenantStatusProvisioning,
		types.TenantStatusActivationAbandoned,
	} {
		t.Run(status, func(t *testing.T) {
			tenant := &types.Tenant{ID: 42, Name: "enterprise", Status: status}
			service := &platformTenantScopeService{tenant: tenant}
			user := &types.User{ID: "platform-admin", IsActive: true, IsSystemAdmin: true}
			router := platformTenantScopeTestRouter(service, user, func(c *gin.Context) {
				ginTenantID, ginOK := c.Get(types.TenantIDContextKey.String())
				contextTenantID, contextOK := types.TenantIDFromContext(c.Request.Context())
				ginTenant, _ := c.Get(types.TenantInfoContextKey.String())
				contextTenant, _ := c.Request.Context().Value(types.TenantInfoContextKey).(*types.Tenant)
				contextUser, _ := c.Request.Context().Value(types.UserContextKey).(*types.User)
				if !ginOK || ginTenantID != uint64(42) || !contextOK || contextTenantID != 42 {
					t.Fatalf("tenant id was not attached to both contexts: gin=%v context=%d", ginTenantID, contextTenantID)
				}
				if ginTenant != tenant || contextTenant != tenant {
					t.Fatal("tenant object was not attached to both contexts")
				}
				if contextUser != user || user.TenantID != 0 || user.CanAccessAllTenants {
					t.Fatal("request-local tenant scope changed the platform identity")
				}
				c.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(http.MethodGet, "/api/v1/system/admin/tenants/42/probe", nil)
			request.Header.Set("X-Tenant-ID", "42")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNoContent, response.Body.String())
			}
			if service.calls != 1 {
				t.Fatalf("tenant lookups = %d, want 1", service.calls)
			}
		})
	}
}

func TestBindSystemAdminTenantScopeRejectsBeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		path        string
		header      string
		systemAdmin bool
		tenant      *types.Tenant
		wantStatus  int
		wantCalls   int
	}{
		{name: "ordinary user", path: "/api/v1/system/admin/tenants/42/probe", tenant: &types.Tenant{ID: 42}, wantStatus: http.StatusForbidden},
		{name: "invalid id", path: "/api/v1/system/admin/tenants/not-an-id/probe", systemAdmin: true, tenant: &types.Tenant{ID: 42}, wantStatus: http.StatusBadRequest},
		{name: "zero id", path: "/api/v1/system/admin/tenants/0/probe", systemAdmin: true, tenant: &types.Tenant{ID: 42}, wantStatus: http.StatusBadRequest},
		{name: "missing tenant", path: "/api/v1/system/admin/tenants/42/probe", systemAdmin: true, wantStatus: http.StatusNotFound, wantCalls: 1},
		{name: "header mismatch", path: "/api/v1/system/admin/tenants/42/probe", header: "41", systemAdmin: true, tenant: &types.Tenant{ID: 42}, wantStatus: http.StatusBadRequest},
		{name: "invalid header", path: "/api/v1/system/admin/tenants/42/probe", header: "invalid", systemAdmin: true, tenant: &types.Tenant{ID: 42}, wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &platformTenantScopeService{tenant: test.tenant}
			user := &types.User{ID: "caller", IsActive: true, IsSystemAdmin: test.systemAdmin}
			reached := false
			router := platformTenantScopeTestRouter(service, user, func(c *gin.Context) {
				reached = true
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.header != "" {
				request.Header.Set("X-Tenant-ID", test.header)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			if reached {
				t.Fatal("protected handler was reached")
			}
			if service.calls != test.wantCalls {
				t.Fatalf("tenant lookups = %d, want %d", service.calls, test.wantCalls)
			}
		})
	}
}

func platformTenantScopeTestRouter(
	tenantService interfaces.TenantService,
	user *types.User,
	h gin.HandlerFunc,
) *gin.Engine {
	router := gin.New()
	router.Use(ErrorHandler())
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.UserContextKey, user)
		ctx = context.WithValue(ctx, types.UserIDContextKey, user.ID)
		ctx = context.WithValue(ctx, types.SystemAdminContextKey, user.IsSystemAdmin)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET(
		"/api/v1/system/admin/tenants/:tenant_id/probe",
		RequireSystemAdmin(nil),
		BindSystemAdminTenantScope(tenantService),
		h,
	)
	return router
}
