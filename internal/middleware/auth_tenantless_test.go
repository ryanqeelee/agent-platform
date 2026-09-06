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

type oldTenantlessTokenUserService struct {
	interfaces.UserService
	user *types.User
}

func (s *oldTenantlessTokenUserService) ValidateToken(context.Context, string) (*types.User, uint64, error) {
	return s.user, 0, nil
}

type activationStatusTenantService struct {
	interfaces.TenantService
	tenant *types.Tenant
}

func (s *activationStatusTenantService) GetTenantByID(context.Context, uint64) (*types.Tenant, error) {
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

func TestTenantlessSystemAdminControlPlaneAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		path        string
		systemAdmin bool
		wantStatus  int
	}{
		{
			name:        "system admin platform key bootstrap",
			path:        "/api/v1/system/admin/api-keys",
			systemAdmin: true,
			wantStatus:  http.StatusNoContent,
		},
		{
			name:        "ordinary tenantless user",
			path:        "/api/v1/system/admin/api-keys",
			systemAdmin: false,
			wantStatus:  http.StatusConflict,
		},
		{
			name:        "similar prefix is tenant scoped",
			path:        "/api/v1/system/admin-foo/api-keys",
			systemAdmin: true,
			wantStatus:  http.StatusConflict,
		},
		{
			name:        "ordinary workspace route still requires workspace",
			path:        "/api/v1/knowledge-bases",
			systemAdmin: true,
			wantStatus:  http.StatusConflict,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &types.User{ID: "operator", IsActive: true, IsSystemAdmin: tt.systemAdmin}
			router := gin.New()
			router.Use(Auth(nil, &oldTenantlessTokenUserService{user: user}, nil, nil, nil))
			router.POST(tt.path, RequireSystemAdmin(nil), func(c *gin.Context) {
				if !types.IsSystemAdminFromContext(c.Request.Context()) {
					t.Fatal("system-admin context was not attached")
				}
				if _, ok := types.TenantIDFromContext(c.Request.Context()); ok {
					t.Fatal("platform control-plane request gained tenant context")
				}
				c.Status(http.StatusNoContent)
			})

			request := httptest.NewRequest(http.MethodPost, tt.path, nil)
			request.Header.Set("Authorization", "Bearer tenantless-access-token")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tt.wantStatus, response.Body.String())
			}
		})
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
			members.seedActive(user.ID, user.TenantID, types.TenantRoleOwner)
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
