package handler

import (
	"bytes"
	"context"
	apprepo "github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type recordingEnterpriseOperationsRepository struct {
	actorUserID, name, description, status string
	tenantID                               uint64
	analysisEnabled                        bool
	seatsTotal                             *int
	storageQuota                           int64
}

func (*recordingEnterpriseOperationsRepository) ValidateSystemAdministrator(context.Context, string) error {
	return nil
}

func (r *recordingEnterpriseOperationsRepository) UpdateForPlatformOperations(
	_ context.Context, actorUserID string, tenantID uint64, name, description, status string,
	analysisEnabled bool, seatsTotal *int, storageQuota int64,
) (*types.Tenant, int64, error) {
	r.actorUserID, r.tenantID, r.name, r.description, r.status = actorUserID, tenantID, name, description, status
	r.seatsTotal, r.storageQuota = seatsTotal, storageQuota
	r.analysisEnabled = analysisEnabled
	return &types.Tenant{
		ID: tenantID, Name: name, Description: description, Status: status,
		AnalysisEnabled: analysisEnabled, SeatsTotal: seatsTotal, StorageQuota: storageQuota, StorageUsed: 256,
	}, 2, nil
}

func enterprisePatchRouter(repository *recordingEnterpriseOperationsRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), types.UserIDContextKey, "system-admin-1"))
		c.Next()
	})
	handler := NewPlatformOperationsHandler(nil, nil, repository, nil, nil, nil, nil, nil, nil)
	router.PATCH("/api/v1/system/admin/operations/enterprises/:tenant_id", handler.UpdateEnterprise)
	return router
}

func TestPlatformOperationsEnterprisePatchPersistsFrozenFields(t *testing.T) {
	repository := &recordingEnterpriseOperationsRepository{}
	body := `{"name":" Acme ","description":"Updated","status":"suspended","analysis_enabled":false,"seats_total":null,"storage_quota":0}`
	request := httptest.NewRequest(http.MethodPatch,
		"/api/v1/system/admin/operations/enterprises/7", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	enterprisePatchRouter(repository).ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "system-admin-1", repository.actorUserID)
	require.Equal(t, uint64(7), repository.tenantID)
	require.Equal(t, "Acme", repository.name)
	require.Equal(t, "Updated", repository.description)
	require.Equal(t, types.TenantStatusSuspended, repository.status)
	require.Nil(t, repository.seatsTotal)
	require.Zero(t, repository.storageQuota)
	require.JSONEq(t, `{"success":true,"data":{"id":7,"name":"Acme","description":"Updated","status":"suspended","analysis_enabled":false,"seats_total":null,"seats_used":2,"storage_quota":0,"storage_used":256,"created_at":"0001-01-01T00:00:00Z","updated_at":"0001-01-01T00:00:00Z"}}`, response.Body.String())
}

func TestPlatformOperationsEnterprisePatchRejectsUnknownOrLifecycleStatus(t *testing.T) {
	for name, body := range map[string]string{
		"unknown":      `{"name":"Acme","description":"","status":"active","analysis_enabled":true,"seats_total":1,"storage_quota":0,"machine_code":"x"}`,
		"provisioning": `{"name":"Acme","description":"","status":"provisioning","analysis_enabled":true,"seats_total":1,"storage_quota":0}`,
		"negative":     `{"name":"Acme","description":"","status":"active","analysis_enabled":true,"seats_total":1,"storage_quota":-1}`,
	} {
		t.Run(name, func(t *testing.T) {
			repository := &recordingEnterpriseOperationsRepository{}
			request := httptest.NewRequest(http.MethodPatch,
				"/api/v1/system/admin/operations/enterprises/7", bytes.NewBufferString(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			enterprisePatchRouter(repository).ServeHTTP(response, request)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Zero(t, repository.tenantID)
		})
	}
}

func TestOperationsEdgeErrorPreservesPendingRevocation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"pending", &interfaces.PlatformOperationsUpstreamError{StatusCode: 503, Detail: "edge_node_disabled_projection_pending"}, 503, "edge_node_disabled_projection_pending"},
		{"revoked-admin", apprepo.ErrMemberActionForbidden, 403, "system administrator authority is no longer active"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(middleware.ErrorHandler())
			router.POST("/disable", func(c *gin.Context) { writeOperationsEdgeError(c, tc.err) })
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/disable", nil))
			require.Equal(t, tc.status, response.Code)
			require.Contains(t, response.Body.String(), tc.message)
		})
	}
}
