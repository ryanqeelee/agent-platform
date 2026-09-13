package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type revocationBindingService struct {
	request interfaces.EdgeNodeRevocationReceipt
}

func (*revocationBindingService) Prepare(context.Context, interfaces.GovernedEdgeBindingPrepareCommand) (*types.GovernedEdgeBinding, error) {
	return nil, nil
}

func (*revocationBindingService) Confirm(context.Context, interfaces.GovernedEdgeBindingConfirmCommand) (*types.GovernedEdgeBinding, error) {
	return nil, nil
}

func (s *revocationBindingService) Revoke(_ context.Context, enterpriseID, edgeNodeID string, revision int64) (*interfaces.EdgeNodeRevocationReceipt, error) {
	s.request = interfaces.EdgeNodeRevocationReceipt{
		EnterpriseID: enterpriseID, EdgeNodeID: edgeNodeID,
		SentControlRevision: revision, AcceptedControlRevision: 3,
		BindingRevision: 9, Status: "superseded",
	}
	return &s.request, nil
}

func TestEdgeNodeRevocationUsesFrozenReceipt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &revocationBindingService{}
	h := NewPlatformOperationsHandler(nil, nil, nil, nil, nil, nil, nil, nil, service)
	router := gin.New()
	router.Use(middleware.ErrorHandler())
	router.POST("/api/v1/system/edge-node-revocations", h.RevokeEdgeNode)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/system/edge-node-revocations",
		bytes.NewBufferString(`{"enterprise_id":"enterprise","edge_node_id":"old-node","control_revision":40}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, int64(40), service.request.SentControlRevision)
	require.JSONEq(t, `{"schema":"EdgeNodeRevocationReceiptV1","enterprise_id":"enterprise","edge_node_id":"old-node","sent_control_revision":40,"accepted_control_revision":3,"binding_revision":9,"status":"superseded"}`, response.Body.String())
}
