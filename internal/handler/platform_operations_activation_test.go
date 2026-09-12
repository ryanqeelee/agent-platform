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
	responseBody                              []byte
	statusCode                                int
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
	responseBody := b.responseBody
	if responseBody == nil {
		responseBody = []byte(`{
		"schema":"EnterpriseActivationV2","activationId":"activation-1","enterpriseId":"enterprise-1",
		"productBaseTenantId":"pb-tenant-7","bindingId":"binding-1","status":"active","lastErrorCode":null,
		"name":"Acme","description":"","seatsTotal":2,"storageQuota":0,
		"initialAdministratorUserId":"owner-1","aiCapabilityPlanVersionId":"plan-1",
		"createdAt":"2026-09-11T00:00:00Z","updatedAt":"2026-09-11T00:00:00Z","completedAt":null
	}`)
	}
	statusCode := b.statusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	return &interfaces.PlatformOperationsResponse{StatusCode: statusCode, ContentType: "application/json", Body: responseBody}, nil
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
	router.GET("/api/v1/system/admin/operations/enterprises/:tenant_id/edge", handler.GetEnterpriseEdge)
	router.POST("/api/v1/system/admin/operations/enterprises/:tenant_id/edge-enrollment-token/rotate", handler.RotateEnterpriseEnrollmentToken)
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

func TestPlatformOperationsActivationPreservesNullableBindingID(t *testing.T) {
	bridge := &recordingOperationsBridge{responseBody: []byte(`{
		"schema":"EnterpriseActivationV2","activationId":"activation-1","enterpriseId":"enterprise-1",
		"productBaseTenantId":null,"bindingId":null,"status":"pending","lastErrorCode":null,
		"name":"Acme","description":"","seatsTotal":2,"storageQuota":0,
		"initialAdministratorUserId":"owner-1","aiCapabilityPlanVersionId":"plan-1",
		"createdAt":"2026-09-11T00:00:00Z","updatedAt":"2026-09-11T00:00:00Z","completedAt":null
	}`)}
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/system/admin/operations/enterprise-activations/activation-1", nil)
	response := httptest.NewRecorder()
	operationsActivationRouter(bridge).ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.JSONEq(t, `{"success":true,"data":{"schema":"EnterpriseActivationV2","activationId":"activation-1","enterpriseId":"enterprise-1","productBaseTenantId":null,"bindingId":null,"status":"pending","lastErrorCode":null,"name":"Acme","description":"","seatsTotal":2,"storageQuota":0,"initialAdministratorUserId":"owner-1","aiCapabilityPlanVersionId":"plan-1","createdAt":"2026-09-11T00:00:00Z","updatedAt":"2026-09-11T00:00:00Z","completedAt":null}}`, response.Body.String())
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

func TestPlatformOperationsEdgeSummaryUsesBoundTenantBridge(t *testing.T) {
	bridge := &recordingOperationsBridge{responseBody: []byte(`{
		"schema":"PlatformEnterpriseEdgeV1","productBaseTenantId":"7",
		"enterpriseId":"tenant-edge-7","bindingId":"binding-edge-7",
		"summary":{"connectionStatus":"online","policyStatus":"not_connected","nodeCount":1,"onlineNodeCount":1,"lastSeenAt":"2026-09-12T00:00:00Z"},
		"nodes":[{"edgeNodeId":"csf-via-rong","displayName":"CSF","version":"v1","status":"online","catalogVersion":"catalog-v1","dataServiceStatus":{"status":"available"},"registeredAt":"2026-09-11T00:00:00Z","lastSeenAt":"2026-09-12T00:00:00Z"}]
	}`)}
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/system/admin/operations/enterprises/7/edge", nil)
	response := httptest.NewRecorder()

	operationsActivationRouter(bridge).ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, http.MethodGet, bridge.method)
	require.Equal(t, "/api/internal/product-base/operations/enterprises/7/edge", bridge.path)
	require.Equal(t, "system-admin-1", bridge.actor)
	require.JSONEq(t, `{"success":true,"data":{"schema":"PlatformEnterpriseEdgeV1","productBaseTenantId":"7","enterpriseId":"tenant-edge-7","bindingId":"binding-edge-7","summary":{"connectionStatus":"online","policyStatus":"not_connected","nodeCount":1,"onlineNodeCount":1,"lastSeenAt":"2026-09-12T00:00:00Z"},"nodes":[{"edgeNodeId":"csf-via-rong","displayName":"CSF","version":"v1","status":"online","catalogVersion":"catalog-v1","dataServiceStatus":{"status":"available"},"registeredAt":"2026-09-11T00:00:00Z","lastSeenAt":"2026-09-12T00:00:00Z"}]}}`, response.Body.String())
}

func TestPlatformOperationsEnrollmentTokenIsBrowserSecretResponse(t *testing.T) {
	bridge := &recordingOperationsBridge{responseBody: []byte(`{
		"schema":"PlatformEnterpriseEdgeEnrollmentV1","productBaseTenantId":"7",
		"enterpriseId":"tenant-edge-7","bindingId":"binding-edge-7",
		"enrollmentToken":"EDGE-once","rotatedAt":"2026-09-12T00:00:00Z"
	}`)}
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/system/admin/operations/enterprises/7/edge-enrollment-token/rotate", nil)
	response := httptest.NewRecorder()

	operationsActivationRouter(bridge).ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, http.MethodPost, bridge.method)
	require.Equal(t, "/api/internal/product-base/operations/enterprises/7/edge-enrollment-token/rotate", bridge.path)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	require.Equal(t, "no-cache", response.Header().Get("Pragma"))
}

func TestPlatformOperationsEdgeRejectsInvalidTenantBeforeBridge(t *testing.T) {
	bridge := &recordingOperationsBridge{}
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/system/admin/operations/enterprises/0/edge", nil)
	response := httptest.NewRecorder()

	operationsActivationRouter(bridge).ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	require.Empty(t, bridge.method)
}

func TestPlatformOperationsEdgeMapsCenterServiceCredentialFailureToUnavailable(t *testing.T) {
	bridge := &recordingOperationsBridge{
		statusCode:   http.StatusUnauthorized,
		responseBody: []byte(`{"detail":"invalid service credential"}`),
	}
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/system/admin/operations/enterprises/7/edge", nil)
	response := httptest.NewRecorder()

	operationsActivationRouter(bridge).ServeHTTP(response, request)

	require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
	require.NotContains(t, response.Body.String(), "credential")
}
