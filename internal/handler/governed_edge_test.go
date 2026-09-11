package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type edgeBindingService struct {
	interfaces.TenantService
	calls int
}

func (s *edgeBindingService) ApplyGovernedEdgeBinding(_ context.Context, tenantID uint64, b types.GovernedEdgeBinding) error {
	s.calls++
	return nil
}

func TestGovernedEdgeBindingRequiresPlatformMachineIdentity(t *testing.T) {
	for _, scope := range []types.APIKeyScopeType{"", types.APIKeyScopeTenant, types.APIKeyScopePlatform} {
		t.Run(string(scope), func(t *testing.T) {
			service := &edgeBindingService{}
			h := &TenantHandler{service: service}
			r := gin.New()
			r.PUT("/system/tenants/:id/edge-binding", h.PutGovernedEdgeBinding)
			req := httptest.NewRequest(http.MethodPut, "/system/tenants/10001/edge-binding", bytes.NewBufferString(`{"binding_id":"b","enterprise_id":"e","revision":1,"enabled":false}`))
			if scope != "" {
				req = req.WithContext(types.WithTenantAPIKeyScope(req.Context(), types.TenantAPIKeyScope{ScopeType: scope}))
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, req)
			if scope == types.APIKeyScopePlatform {
				require.Equal(t, 200, response.Code)
				require.Equal(t, 1, service.calls)
			} else {
				require.Equal(t, 403, response.Code)
				require.Zero(t, service.calls)
			}
		})
	}
}
