package handler

import (
	"bytes"
	"context"
	"encoding/json"
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

type activationHandlerTenantService struct {
	interfaces.TenantService
	command interfaces.EnterpriseActivationCommand
	result  *interfaces.EnterpriseActivationResult
	err     error
}

func (s *activationHandlerTenantService) ApplyEnterpriseActivation(
	_ context.Context,
	command interfaces.EnterpriseActivationCommand,
) (*interfaces.EnterpriseActivationResult, error) {
	s.command = command
	return s.result, s.err
}

func activationHandlerRouter(service interfaces.TenantService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	h := &TenantHandler{service: service}
	r.PUT("/system/enterprise-activations/:activation_id", h.PutEnterpriseActivation)
	return r
}

func TestPutEnterpriseActivationReturnsStableProjection(t *testing.T) {
	service := &activationHandlerTenantService{result: &interfaces.EnterpriseActivationResult{
		ActivationID: "activation-1", TenantID: 10001, OwnerMembershipID: 23,
		RequestSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		State:         types.EnterpriseActivationStatePrepared,
	}}
	body := `{
		"schema":"ProductBaseTenantOwnerActivationV1",
		"requestSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"tenant":{"name":"Acme","description":"Acme workspace"},
		"firstOwnerUserId":"owner-1",
		"desiredState":"prepared"
	}`
	request := httptest.NewRequest(http.MethodPut, "/system/enterprise-activations/activation-1", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	activationHandlerRouter(service).ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "activation-1", service.command.ActivationID)
	require.Equal(t, "Acme", service.command.TenantName)
	require.Equal(t, "owner-1", service.command.FirstOwnerUserID)

	var got productBaseTenantOwnerActivationResponseV1
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &got))
	require.Equal(t, "ProductBaseTenantOwnerActivationV1", got.Schema)
	require.Equal(t, uint64(10001), got.TenantID)
	require.Equal(t, uint64(23), got.OwnerMembershipID)
	require.Equal(t, types.EnterpriseActivationStatePrepared, got.State)
}

func TestPutEnterpriseActivationRejectsUnknownFieldsAndPreservesTypedConflict(t *testing.T) {
	t.Run("unknown field", func(t *testing.T) {
		service := &activationHandlerTenantService{}
		body := `{"schema":"ProductBaseTenantOwnerActivationV1","enterpriseId":"not-owned-by-pb","requestSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","tenant":{"name":"Acme","description":""},"firstOwnerUserId":"owner-1","desiredState":"prepared"}`
		response := httptest.NewRecorder()
		activationHandlerRouter(service).ServeHTTP(response,
			httptest.NewRequest(http.MethodPut, "/system/enterprise-activations/activation-1", bytes.NewBufferString(body)))
		require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	})

	t.Run("typed conflict", func(t *testing.T) {
		service := &activationHandlerTenantService{err: apperrors.NewConflictError("activation conflict")}
		body := `{"schema":"ProductBaseTenantOwnerActivationV1","requestSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","tenant":{"name":"Acme","description":""},"firstOwnerUserId":"owner-1","desiredState":"active"}`
		response := httptest.NewRecorder()
		activationHandlerRouter(service).ServeHTTP(response,
			httptest.NewRequest(http.MethodPut, "/system/enterprise-activations/activation-1", bytes.NewBufferString(body)))
		require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	})
}
