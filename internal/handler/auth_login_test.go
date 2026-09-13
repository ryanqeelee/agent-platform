package handler

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type loginUserServiceStub struct {
	interfaces.UserService
	response *types.LoginResponse
	err      error
}

func (s *loginUserServiceStub) Login(context.Context, *types.LoginRequest) (*types.LoginResponse, error) {
	return s.response, s.err
}

func loginTestRequest(t *testing.T, service interfaces.UserService) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.POST("/auth/login", (&AuthHandler{userService: service}).Login)
	request := httptest.NewRequest(
		http.MethodPost,
		"/auth/login",
		bytes.NewBufferString(`{"email":"user@example.invalid","password":"CorrectHorse9"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestLoginRepositoryFailureReturnsGenericServiceUnavailable(t *testing.T) {
	response := loginTestRequest(t, &loginUserServiceStub{
		err: stderrors.New("database disk is full"),
	})

	require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
	require.NotContains(t, response.Body.String(), "database disk is full")
	var body struct {
		Success bool `json:"success"`
		Error   struct {
			Code    apperrors.ErrorCode `json:"code"`
			Message string              `json:"message"`
			Details any                 `json:"details"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.False(t, body.Success)
	require.Equal(t, apperrors.ErrServiceUnavailable, body.Error.Code)
	require.Equal(t, "Service temporarily unavailable", body.Error.Message)
	require.Nil(t, body.Error.Details)
}

func TestLoginCredentialFailureRemainsUnauthorized(t *testing.T) {
	response := loginTestRequest(t, &loginUserServiceStub{
		response: &types.LoginResponse{
			Success: false,
			Message: "Invalid email or password",
		},
	})

	require.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "Invalid email or password")
}
