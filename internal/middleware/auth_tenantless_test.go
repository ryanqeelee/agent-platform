package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type oldTenantlessTokenUserService struct {
	interfaces.UserService
	user *types.User
}

func (s *oldTenantlessTokenUserService) ValidateToken(context.Context, string) (*types.User, uint64, error) {
	return s.user, 0, nil
}

func (s *oldTenantlessTokenUserService) ValidateIdentityToken(context.Context, string) (*types.User, uint64, error) {
	return s.user, 0, nil
}

type activationStatusTenantService struct {
	interfaces.TenantService
	tenant *types.Tenant
	calls  int
}

func (s *activationStatusTenantService) GetTenantByID(context.Context, uint64) (*types.Tenant, error) {
	s.calls++
	return s.tenant, nil
}

func TestTenantOptionalAPISurface(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodGet, "/api/v1/auth/me", true},
		{http.MethodPut, "/api/v1/auth/me", true},
		{http.MethodPut, "/api/v1/auth/me/preferences", true},
		{http.MethodPost, "/api/v1/tenants", true},
		{http.MethodGet, "/api/v1/me/invitations", true},
		{http.MethodPost, "/api/v1/me/invitations/12/accept", true},
		{http.MethodGet, "/api/v1/knowledge-bases", false},
		{http.MethodGet, "/api/v1/tenants", false},
	}
	for _, tt := range tests {
		if got := isTenantOptionalAPI(tt.path, tt.method); got != tt.want {
			t.Errorf("isTenantOptionalAPI(%s %s) = %v, want %v", tt.method, tt.path, got, tt.want)
		}
	}
}

func TestInvitationAcceptanceIdentityAPISurfaceIsExact(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodPost, "/api/v1/me/invitations/12/accept", true},
		{http.MethodPost, "/api/v1/me/invitations/accept-by-token", true},
		{http.MethodGet, "/api/v1/me/invitations/12/accept", false},
		{http.MethodPost, "/api/v1/me/invitations/12/decline", false},
		{http.MethodGet, "/api/v1/me/invitations", false},
		{http.MethodPost, "/api/v1/me/invitations/not-an-id/accept", false},
		{http.MethodPost, "/api/v1/me/invitations/12/accept/extra", false},
		{http.MethodPost, "/api/v1/tenants/12/invitations/34/accept", false},
	}
	for _, tt := range tests {
		if got := isInvitationAcceptanceIdentityAPI(tt.path, tt.method); got != tt.want {
			t.Errorf("isInvitationAcceptanceIdentityAPI(%s %s) = %v, want %v", tt.method, tt.path, got, tt.want)
		}
	}
}

func TestTenantlessSystemAdminControlPlaneAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		method      string
		path        string
		systemAdmin bool
		wantStatus  int
	}{
		{
			name:        "system admin platform key bootstrap",
			method:      http.MethodPost,
			path:        "/api/v1/system/admin/api-keys",
			systemAdmin: true,
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "ordinary tenantless user",
			method:      http.MethodPost,
			path:        "/api/v1/system/admin/api-keys",
			systemAdmin: false,
			wantStatus:  http.StatusConflict,
		},
		{
			name:        "similar prefix is tenant scoped",
			method:      http.MethodPost,
			path:        "/api/v1/system/admin-foo/api-keys",
			systemAdmin: true,
			wantStatus:  http.StatusConflict,
		},
		{
			name:        "ordinary workspace route still requires workspace",
			method:      http.MethodPost,
			path:        "/api/v1/knowledge-bases",
			systemAdmin: true,
			wantStatus:  http.StatusConflict,
		},
		{
			name:        "model list is a tenantless platform operation",
			method:      http.MethodGet,
			path:        "/api/v1/models",
			systemAdmin: true,
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "model credentials remain a tenantless platform operation",
			method:      http.MethodPut,
			path:        "/api/v1/models/model-1/credentials",
			systemAdmin: true,
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "model connection check is a tenantless platform operation",
			method:      http.MethodPost,
			path:        "/api/v1/initialization/remote/check",
			systemAdmin: true,
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "workspace WeKnora Cloud status stays tenant scoped",
			method:      http.MethodGet,
			path:        "/api/v1/models/weknoracloud/status",
			systemAdmin: true,
			wantStatus:  http.StatusConflict,
		},
		{
			name:        "enterprise model runtime projection stays tenant scoped",
			method:      http.MethodGet,
			path:        "/api/v1/platform/model-runtime-settings",
			systemAdmin: true,
			wantStatus:  http.StatusConflict,
		},
		{
			name:        "unrelated platform settings stay tenant scoped",
			method:      http.MethodGet,
			path:        "/api/v1/platform/retrieval-processing-settings",
			systemAdmin: true,
			wantStatus:  http.StatusConflict,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &types.User{ID: "operator", IsActive: true, IsSystemAdmin: tt.systemAdmin}
			router := gin.New()
			router.Use(Auth(nil, &oldTenantlessTokenUserService{user: user}, nil, nil, nil))
			router.Handle(tt.method, tt.path, RequireSystemAdmin(nil), func(c *gin.Context) {
				if !types.IsSystemAdminFromContext(c.Request.Context()) {
					t.Fatal("system-admin context was not attached")
				}
				if _, ok := types.TenantIDFromContext(c.Request.Context()); ok {
					t.Fatal("platform control-plane request gained tenant context")
				}
				c.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(tt.method, tt.path, nil)
			request.Header.Set("Authorization", "Bearer tenantless-access-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tt.wantStatus, response.Body.String())
			}
		})
	}
}

func TestPlatformOperationsSystemAdminUsesIdentityOnlyContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := &types.User{ID: "operator", TenantID: 7, IsActive: true, IsSystemAdmin: true}
	tenantService := &activationStatusTenantService{tenant: &types.Tenant{
		ID: user.TenantID, Status: types.TenantStatusSuspended,
	}}

	for _, tc := range []struct {
		path       string
		wantStatus int
	}{
		{path: "/api/v1/system/admin/operations/enterprises", wantStatus: http.StatusNoContent},
		{path: "/api/v1/system/admin/api-keys", wantStatus: http.StatusNoContent},
		{path: "/api/v1/system/admin/operations-legacy/enterprises", wantStatus: http.StatusNoContent},
		{path: "/api/v1/knowledge-bases", wantStatus: http.StatusConflict},
	} {
		t.Run(tc.path, func(t *testing.T) {
			router := gin.New()
			router.Use(Auth(tenantService, &oldTenantlessTokenUserService{user: user}, nil, nil, nil))
			router.GET(tc.path, RequireSystemAdmin(nil), func(c *gin.Context) {
				if _, ok := types.TenantIDFromContext(c.Request.Context()); ok {
					t.Fatal("platform operations request gained tenant context")
				}
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, tc.path, nil)
			request.Header.Set("Authorization", "Bearer system-admin-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tc.wantStatus, response.Body.String())
			}
		})
	}
}

func TestLegacySystemAdminIgnoresEnterpriseContextBeforeTenantResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	user := &types.User{ID: "operator", TenantID: 7, IsActive: true, IsSystemAdmin: true}
	tenantService := &activationStatusTenantService{tenant: &types.Tenant{
		ID: user.TenantID, Status: types.TenantStatusSuspended,
	}}
	router := gin.New()
	router.Use(Auth(tenantService, &oldTenantlessTokenUserService{user: user}, nil, nil, nil))
	router.GET("/api/v1/auth/me", func(c *gin.Context) {
		if _, ok := types.TenantIDFromContext(c.Request.Context()); ok {
			t.Fatal("auth/me gained tenant context from legacy identity")
		}
		if !types.IsSystemAdminFromContext(c.Request.Context()) {
			t.Fatal("auth/me lost platform authority")
		}
		c.Status(http.StatusNoContent)
	})
	enterpriseHandlerCalled := false
	router.GET("/api/v1/knowledge-bases", func(c *gin.Context) {
		enterpriseHandlerCalled = true
		if _, ok := types.TenantIDFromContext(c.Request.Context()); ok {
			t.Fatal("enterprise API gained tenant context")
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.Header.Set("Authorization", "Bearer legacy-system-admin-token")
	request.Header.Set("X-Tenant-ID", "7")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("auth/me status=%d body=%s", response.Code, response.Body.String())
	}
	if tenantService.calls != 0 {
		t.Fatalf("auth/me resolved tenant %d time(s), want 0", tenantService.calls)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/knowledge-bases", nil)
	request.Header.Set("Authorization", "Bearer legacy-system-admin-token")
	request.Header.Set("X-Tenant-ID", "7")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "TENANT_REQUIRED") {
		t.Fatalf("enterprise API status=%d body=%s", response.Code, response.Body.String())
	}
	if enterpriseHandlerCalled {
		t.Fatal("tenant-required enterprise handler was reached")
	}
	if tenantService.calls != 0 {
		t.Fatalf("enterprise API resolved tenant %d time(s), want 0", tenantService.calls)
	}
}

func TestAuthRejectsOldTenantlessJWTAfterActivationBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []string{
		types.TenantStatusProvisioning,
		types.TenantStatusActivationAbandoned,
		types.TenantStatusActive,
	} {
		t.Run(status, func(t *testing.T) {
			user := &types.User{ID: "owner", TenantID: 7, IsActive: true}
			members := newFakeMemberService()
			// Keep membership usable so tenant status alone decides this old-token path.
			members.seedActive(user.ID, user.TenantID, types.TenantRoleAdmin)
			router := gin.New()
			router.Use(Auth(
				&activationStatusTenantService{tenant: &types.Tenant{ID: user.TenantID, Status: status}},
				&oldTenantlessTokenUserService{user: user}, members, nil, cfgWithRBAC(true),
			))
			router.GET("/api/v1/auth/me", func(c *gin.Context) { c.Status(http.StatusNoContent) })

			request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
			request.Header.Set("Authorization", "Bearer old-tenantless-access-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			want := http.StatusForbidden
			if status == types.TenantStatusActive {
				want = http.StatusNoContent
			}
			if response.Code != want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, want, response.Body.String())
			}
		})
	}
}
