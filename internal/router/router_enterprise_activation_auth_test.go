package router

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/handler"
	"github.com/Tencent/WeKnora/internal/middleware"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type activationRouteTenantService struct {
	interfaces.TenantService
	calls int
}

func (s *activationRouteTenantService) ApplyEnterpriseActivation(
	_ context.Context,
	command interfaces.EnterpriseActivationCommand,
) (*interfaces.EnterpriseActivationResult, error) {
	s.calls++
	return &interfaces.EnterpriseActivationResult{
		ActivationID:      command.ActivationID,
		TenantID:          10001,
		OwnerMembershipID: 7,
		RequestSHA256:     command.RequestSHA256,
		State:             types.EnterpriseActivationStatePrepared,
	}, nil
}

func enterpriseActivationRouteHarness(scope *types.TenantAPIKeyScope, service interfaces.TenantService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		if scope == nil {
			ctx = context.WithValue(ctx, types.UserContextKey, &types.User{
				ID: "ordinary-user", TenantID: 42, IsActive: true,
			})
		} else {
			ctx = types.WithTenantAPIKeyScope(ctx, *scope)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	guards := &rbacGuards{}
	v1 := engine.Group("/api/v1")
	v1.Use(guards.ensureAPIKeyAuthorizer().Middleware())
	tenantHandler := handler.NewTenantHandler(service, nil, nil, nil, nil, nil, nil, nil)
	RegisterTenantRoutes(
		v1,
		tenantHandler,
		&handler.TenantMemberHandler{},
		&handler.TenantInvitationHandler{},
		nil,
		guards,
	)
	return engine
}

func TestEnterpriseActivationRouteRequiresManagingPlatformAPIKey(t *testing.T) {
	tests := []struct {
		name       string
		scope      *types.TenantAPIKeyScope
		wantStatus int
		wantCalls  int
	}{
		{name: "ordinary JWT", wantStatus: http.StatusForbidden},
		{
			name: "workspace API key",
			scope: &types.TenantAPIKeyScope{
				ScopeType:    types.APIKeyScopeTenant,
				Capabilities: types.StringArray{string(types.APIKeyCapabilitySystemTenantsManage)},
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "platform API key without capability",
			scope: &types.TenantAPIKeyScope{
				ScopeType:    types.APIKeyScopePlatform,
				Capabilities: types.StringArray{string(types.APIKeyCapabilitySystemTenantsRead)},
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "platform API key full access without capability",
			scope: &types.TenantAPIKeyScope{
				ScopeType:  types.APIKeyScopePlatform,
				FullAccess: true,
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "managing platform API key",
			scope: &types.TenantAPIKeyScope{
				ScopeType:    types.APIKeyScopePlatform,
				Capabilities: types.StringArray{string(types.APIKeyCapabilitySystemTenantsManage)},
			},
			wantStatus: http.StatusOK,
			wantCalls:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &activationRouteTenantService{}
			engine := enterpriseActivationRouteHarness(tt.scope, service)
			body := `{
				"schema":"ProductBaseTenantOwnerActivationV1",
				"requestSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"tenant":{"name":"Acme","description":""},
				"firstOwnerUserId":"owner-1",
				"desiredState":"prepared"
			}`
			request := httptest.NewRequest(
				http.MethodPut,
				"/api/v1/system/enterprise-activations/activation-1",
				bytes.NewBufferString(body),
			)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)

			require.Equal(t, tt.wantStatus, response.Code, response.Body.String())
			require.Equal(t, tt.wantCalls, service.calls)
		})
	}
}
