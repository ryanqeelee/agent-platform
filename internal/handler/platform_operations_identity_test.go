package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type recordingPlatformIdentityService struct {
	actorUserID string
	commandID   string
	request     *types.AdminCreateUserRequest
	resetErr    error
}

func (s *recordingPlatformIdentityService) CreateInitialAdministrator(
	_ context.Context, actorUserID, commandID string, request *types.AdminCreateUserRequest,
) (*types.PlatformInitialAdministratorResult, error) {
	s.actorUserID, s.commandID, s.request = actorUserID, commandID, request
	return &types.PlatformInitialAdministratorResult{
		CommandID: commandID, UserID: "initial-admin-1", Username: request.Username,
		Email: request.Email, Status: "created", Replayed: false,
	}, nil
}

func (s *recordingPlatformIdentityService) GetInitialAdministrator(
	_ context.Context, actorUserID, commandID string,
) (*types.PlatformInitialAdministratorResult, error) {
	s.actorUserID, s.commandID = actorUserID, commandID
	return &types.PlatformInitialAdministratorResult{
		CommandID: commandID, UserID: "initial-admin-1", Username: "owner",
		Email: "owner@example.invalid", Status: "created", Replayed: true,
	}, nil
}

func (s *recordingPlatformIdentityService) ResetEnterpriseMemberPassword(
	context.Context, string, uint64, string, string,
) error {
	return s.resetErr
}

func platformIdentityRouter(identities *recordingPlatformIdentityService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), types.UserIDContextKey, "system-admin-1"))
		c.Next()
	})
	handler := NewPlatformOperationsHandler(nil, nil, nil, nil, identities, nil)
	router.POST("/api/v1/system/admin/operations/initial-administrators", handler.CreateInitialAdministrator)
	router.GET("/api/v1/system/admin/operations/initial-administrators/:command_id", handler.GetInitialAdministrator)
	router.POST("/api/v1/system/admin/operations/enterprises/:tenant_id/members/:user_id/password-reset", handler.ResetMemberPassword)
	return router
}

func TestPlatformOperationsPasswordResetMapsEnterpriseLifecycleConflict(t *testing.T) {
	identities := &recordingPlatformIdentityService{resetErr: service.ErrEnterpriseNotActive}
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/system/admin/operations/enterprises/7/members/employee-1/password-reset",
		bytes.NewBufferString(`{"new_password":"NewPassword456!"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	platformIdentityRouter(identities).ServeHTTP(response, request)
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
}

func TestPlatformOperationsInitialAdministratorStableReceiptContract(t *testing.T) {
	identities := &recordingPlatformIdentityService{}
	router := platformIdentityRouter(identities)

	post := httptest.NewRequest(http.MethodPost, "/api/v1/system/admin/operations/initial-administrators",
		bytes.NewBufferString(`{"username":"owner","email":"owner@example.invalid","password":"Password123!"}`))
	post.Header.Set("Content-Type", "application/json")
	post.Header.Set("Idempotency-Key", "initial-admin-command-1")
	postResponse := httptest.NewRecorder()
	router.ServeHTTP(postResponse, post)
	require.Equal(t, http.StatusCreated, postResponse.Code, postResponse.Body.String())
	require.Equal(t, "system-admin-1", identities.actorUserID)
	require.Equal(t, "initial-admin-command-1", identities.commandID)
	require.NotNil(t, identities.request.Password)
	require.JSONEq(t, `{"success":true,"data":{"command_id":"initial-admin-command-1","user_id":"initial-admin-1","username":"owner","email":"owner@example.invalid","status":"created","replayed":false}}`, postResponse.Body.String())

	get := httptest.NewRequest(http.MethodGet,
		"/api/v1/system/admin/operations/initial-administrators/initial-admin-command-1", nil)
	getResponse := httptest.NewRecorder()
	router.ServeHTTP(getResponse, get)
	require.Equal(t, http.StatusOK, getResponse.Code, getResponse.Body.String())
	require.JSONEq(t, `{"success":true,"data":{"command_id":"initial-admin-command-1","user_id":"initial-admin-1","username":"owner","email":"owner@example.invalid","status":"created","replayed":true}}`, getResponse.Body.String())
}

func TestPlatformOperationsInitialAdministratorRequiresIdempotencyKeyAndPassword(t *testing.T) {
	for name, tc := range map[string]struct {
		key  string
		body string
	}{
		"missing key":      {body: `{"username":"owner","email":"owner@example.invalid","password":"Password123!"}`},
		"missing password": {key: "command-1", body: `{"username":"owner","email":"owner@example.invalid"}`},
		"unknown field":    {key: "command-1", body: `{"username":"owner","email":"owner@example.invalid","password":"Password123!","tenant_id":7}`},
	} {
		t.Run(name, func(t *testing.T) {
			identities := &recordingPlatformIdentityService{}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/system/admin/operations/initial-administrators", bytes.NewBufferString(tc.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", tc.key)
			response := httptest.NewRecorder()
			platformIdentityRouter(identities).ServeHTTP(response, request)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Empty(t, identities.commandID)
		})
	}
}
