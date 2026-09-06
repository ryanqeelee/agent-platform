package session

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEmployeeAssistantNeverResolvesFromSharedTenant(t *testing.T) {
	local := &types.CustomAgent{ID: types.BuiltinEmployeeAssistantID, TenantID: 7}
	h := &Handler{customAgentService: &resolveOwnAgentStub{agent: local}, agentShareService: &resolveAgentShareStub{
		agent: &types.CustomAgent{ID: types.BuiltinEmployeeAssistantID, TenantID: 84},
	}}
	c := &gin.Context{}
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	agent, tenant, shared := h.resolveAgent(ctx, c, types.BuiltinEmployeeAssistantID, 0)
	require.Same(t, local, agent)
	require.Zero(t, tenant)
	require.False(t, shared)
	agent, tenant, shared = h.resolveAgent(ctx, c, types.BuiltinEmployeeAssistantID, 84)
	require.Nil(t, agent)
	require.Zero(t, tenant)
	require.False(t, shared)
}
