package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

type suspendedIssuanceUserService struct {
	interfaces.UserService
	user *types.User
}

func (s *suspendedIssuanceUserService) GetUserByEmail(context.Context, string) (*types.User, error) {
	return s.user, nil
}

func (s *suspendedIssuanceUserService) GenerateTokens(context.Context, *types.User) (string, string, error) {
	return "", "", service.ErrMembershipSuspended
}

func (s *suspendedIssuanceUserService) RefreshToken(context.Context, string) (string, string, error) {
	return "", "", service.ErrMembershipSuspended
}

func TestSuspendedMembershipAuthHandlersReturnUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	users := &suspendedIssuanceUserService{
		user: &types.User{ID: "suspended-user", TenantID: 42, IsActive: true},
	}
	h := &AuthHandler{userService: users}

	t.Run("refresh", func(t *testing.T) {
		router := gin.New()
		router.Use(errorCapture())
		router.POST("/auth/refresh", h.RefreshToken)
		request := httptest.NewRequest(
			http.MethodPost,
			"/auth/refresh",
			bytes.NewBufferString(`{"refreshToken":"suspended-refresh"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusUnauthorized ||
			!bytes.Contains(response.Body.Bytes(), []byte("Workspace membership is suspended")) {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("Lite AutoSetup", func(t *testing.T) {
		previousEdition := Edition
		Edition = "lite"
		t.Cleanup(func() { Edition = previousEdition })

		router := gin.New()
		router.Use(errorCapture())
		router.POST("/auth/auto-setup", h.AutoSetup)
		response := httptest.NewRecorder()
		router.ServeHTTP(
			response,
			httptest.NewRequest(http.MethodPost, "/auth/auto-setup", nil),
		)

		if response.Code != http.StatusUnauthorized ||
			!bytes.Contains(response.Body.Bytes(), []byte("Workspace membership is suspended")) {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	})
}
