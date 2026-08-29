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
