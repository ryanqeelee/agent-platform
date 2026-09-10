package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type governedTokenUsers struct{ interfaces.UserService }

func (governedTokenUsers) ValidateToken(_ context.Context, token string) (*types.User, uint64, error) {
	if token != "valid-user-jwt" {
		return nil, 0, fmt.Errorf("invalid token")
	}
	return &types.User{ID: "user-a", TenantID: 10001, IsActive: true}, 10001, nil
}

func TestGovernedDataCredentialOnlyAfterJWTAdmission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	members := newFakeMemberService()
	members.seedActive("user-a", 10001, types.TenantRoleViewer)
	tenant := &activationStatusTenantService{tenant: &types.Tenant{ID: 10001, Status: types.TenantStatusActive}}
	for _, tc := range []struct {
		name, token, tenantHeader string
		status                    int
	}{
		{"valid", "valid-user-jwt", "10001", 204},
		{"invalid JWT", "expired-jwt", "10001", 401},
		{"no JWT", "", "10001", 401},
		{"foreign tenant", "valid-user-jwt", "20002", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(Auth(tenant, governedTokenUsers{}, members, nil, cfgWithRBAC(true)))
			router.POST("/api/v1/sessions/test", func(c *gin.Context) {
				token, tenantID, ok := types.GovernedDataUserCredential(c.Request.Context())
				if !ok || token != "valid-user-jwt" || tenantID != 10001 {
					t.Error("validated turn credential absent")
				}
				c.Status(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/test", nil)
			if tc.token != "" {
				request.Header.Set("Authorization", "Bearer "+tc.token)
			}
			request.Header.Set("X-Tenant-ID", tc.tenantHeader)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("HTTP %d want %d: %s", response.Code, tc.status, response.Body.String())
			}
		})
	}
}
