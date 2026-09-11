package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type recordingOperationsBridge struct {
	method, path, actor, idempotencyKey, body string
}

func (b *recordingOperationsBridge) Do(
	_ context.Context, method, path, actor, idempotencyKey string, body io.Reader,
) (*interfaces.PlatformOperationsResponse, error) {
	b.method, b.path, b.actor, b.idempotencyKey = method, path, actor, idempotencyKey
	var data []byte
	if body != nil {
		data, _ = io.ReadAll(body)
	}
	b.body = string(data)
	return &interfaces.PlatformOperationsResponse{StatusCode: http.StatusOK, ContentType: "application/json", Body: []byte(`{
		"schema":"EnterpriseActivationV2","activationId":"activation-1","enterpriseId":"enterprise-1",
		"productBaseTenantId":"pb-tenant-7","bindingId":"binding-1","status":"active","lastErrorCode":null,
		"name":"Acme","description":"","seatsTotal":2,"storageQuota":0,
		"initialAdministratorUserId":"owner-1","aiCapabilityPlanVersionId":"plan-1",
		"createdAt":"2026-09-11T00:00:00Z","updatedAt":"2026-09-11T00:00:00Z","completedAt":null
	}`)}, nil
}

func operationsActivationRouter(bridge interfaces.PlatformOperationsBridge) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), types.UserIDContextKey, "system-admin-1")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	handler := NewPlatformOperationsHandler(nil, nil, nil, nil, nil, bridge)
	router.PUT("/api/v1/system/admin/operations/enterprise-activations/:activation_id", handler.ProxyEnterpriseActivation)
	router.GET("/api/v1/system/admin/operations/enterprise-activations/:activation_id", handler.GetEnterpriseActivation)
	return router
}

func TestPlatformOperationsActivationForwardsSimpleDTOAndStableIdempotencyKey(t *testing.T) {
	bridge := &recordingOperationsBridge{}
	body := `{"name":"Acme","description":"","seats_total":2,"storage_quota":0,"initial_administrator_user_id":"owner-1"}`
	request := httptest.NewRequest(http.MethodPut,
		"/api/v1/system/admin/operations/enterprise-activations/activation-1", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "activation-1")
	response := httptest.NewRecorder()
	operationsActivationRouter(bridge).ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, http.MethodPut, bridge.method)
	require.Equal(t, "/api/internal/product-base/operations/enterprise-activations/activation-1", bridge.path)
	require.Equal(t, "system-admin-1", bridge.actor)
	require.Equal(t, "activation-1", bridge.idempotencyKey)
	require.JSONEq(t, body, bridge.body)
	require.JSONEq(t, `{"success":true,"data":{"schema":"EnterpriseActivationV2","activationId":"activation-1","enterpriseId":"enterprise-1","productBaseTenantId":"pb-tenant-7","bindingId":"binding-1","status":"active","lastErrorCode":null,"name":"Acme","description":"","seatsTotal":2,"storageQuota":0,"initialAdministratorUserId":"owner-1","aiCapabilityPlanVersionId":"plan-1","createdAt":"2026-09-11T00:00:00Z","updatedAt":"2026-09-11T00:00:00Z","completedAt":null}}`, response.Body.String())
}

func TestPlatformOperationsActivationGetUsesCenterStringFixture(t *testing.T) {
	bridge := &recordingOperationsBridge{}
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/system/admin/operations/enterprise-activations/activation-1", nil)
	response := httptest.NewRecorder()
	operationsActivationRouter(bridge).ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, http.MethodGet, bridge.method)
	require.Equal(t, "/api/internal/product-base/operations/enterprise-activations/activation-1", bridge.path)
	require.Empty(t, bridge.body)
	require.JSONEq(t, `{"success":true,"data":{"schema":"EnterpriseActivationV2","activationId":"activation-1","enterpriseId":"enterprise-1","productBaseTenantId":"pb-tenant-7","bindingId":"binding-1","status":"active","lastErrorCode":null,"name":"Acme","description":"","seatsTotal":2,"storageQuota":0,"initialAdministratorUserId":"owner-1","aiCapabilityPlanVersionId":"plan-1","createdAt":"2026-09-11T00:00:00Z","updatedAt":"2026-09-11T00:00:00Z","completedAt":null}}`, response.Body.String())
}

func TestPlatformOperationsActivationRejectsCallbackDTO(t *testing.T) {
	bridge := &recordingOperationsBridge{}
	body := `{"schema":"ProductBaseTenantOwnerActivationV2","name":"Acme","description":"","seats_total":2,"storage_quota":0,"initial_administrator_user_id":"owner-1"}`
	request := httptest.NewRequest(http.MethodPut,
		"/api/v1/system/admin/operations/enterprise-activations/activation-1", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "activation-1")
	response := httptest.NewRecorder()
	operationsActivationRouter(bridge).ServeHTTP(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	require.Empty(t, bridge.method)
}
