package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type governedTestUsers struct{ interfaces.UserService }

func (governedTestUsers) ValidateToken(context.Context, string) (*types.User, uint64, error) {
	// Home tenant differs from the authenticated active workspace.
	return &types.User{ID: "user-a", TenantID: 999, IsActive: true}, 10001, nil
}

type governedTestResolver struct{ connection types.GovernedEdgeConnection }

func (r *governedTestResolver) Resolve(_ context.Context, tenant uint64) (types.GovernedEdgeConnection, error) {
	if tenant != 10001 {
		return types.GovernedEdgeConnection{}, tools.ErrGovernedDataAccessDenied
	}
	return r.connection, nil
}

type governedTestMembers struct {
	interfaces.TenantMemberService
	allowed bool
}

func (m *governedTestMembers) GetMembership(_ context.Context, user string, tenant uint64) (*types.TenantMember, error) {
	return &types.TenantMember{UserID: user, TenantID: tenant, Status: types.TenantMemberStatusActive, OperatingAnalysisAccess: m.allowed}, nil
}

func TestGovernedDataRegistrationUsesCurrentUser(t *testing.T) {
	calls := 0
	edge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "/v1/catalog", r.URL.Path)
		require.Equal(t, "Bearer edge-secret", r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("X-Tenant-ID"))
		require.Equal(t, "enterprise", r.URL.Query().Get("enterprise_id"))
		fmt.Fprint(w, `{"contract_version":"edge-governed-query-v1","enterprise_id":"enterprise","edge_node_id":"edge","catalog":{"version":"v1","freshness_token":"fresh"},"source":{"source_id":"retail"}}`)
	}))
	defer edge.Close()
	members := &governedTestMembers{allowed: true}
	resolver := &governedTestResolver{connection: types.GovernedEdgeConnection{EnterpriseID: "enterprise", EdgeNodeID: "edge", SourceID: "retail", BaseURL: edge.URL, Token: "edge-secret"}}
	s := &agentService{userService: governedTestUsers{}, tenantMemberService: members, governedEdgeResolver: resolver}
	ctx := operatingReadContext("user-a", 10001)
	bound := types.WithGovernedDataUserCredential(ctx, "user-jwt")
	selected := &types.AgentConfig{AllowedTools: []string{tools.ToolGovernedDataSchema, tools.ToolGovernedDataQuery}, EmployeeAssistant: true}
	for _, tc := range []struct {
		name                string
		ctx                 context.Context
		cfg                 *types.AgentConfig
		rejects, registered bool
	}{
		{"selected", bound, selected, false, true},
		{"ordinary", bound, &types.AgentConfig{}, false, false},
		{"no interactive credential", ctx, selected, true, false},
		{"shared tenant", context.WithValue(bound, types.TenantIDContextKey, uint64(20002)), selected, true, false},
		{"query without schema", bound, &types.AgentConfig{AllowedTools: []string{tools.ToolGovernedDataQuery}}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := tools.NewToolRegistry()
			err := s.registerGovernedDataTools(tc.ctx, registry, tc.cfg, "native-session")
			require.Equal(t, tc.rejects, err != nil)
			tool, err := registry.GetTool(tools.ToolGovernedDataSchema)
			require.Equal(t, tc.registered, err == nil)
			if tc.registered {
				_, err = tool.Execute(tc.ctx, json.RawMessage(`{}`))
				require.NoError(t, err)
			}
		})
	}
	require.Equal(t, 1, calls)
	members.allowed = false
	_, err := AuthorizeGovernedData(bound, governedTestUsers{}, members, resolver)
	require.ErrorIs(t, err, tools.ErrGovernedDataAccessDenied)
	members.allowed = true
	registry := tools.NewToolRegistry()
	require.NoError(t, s.registerGovernedDataTools(bound, registry, selected, "native-session"))
	resolver.connection.EdgeNodeID = "rebound"
	tool, _ := registry.GetTool(tools.ToolGovernedDataSchema)
	_, err = tool.Execute(bound, json.RawMessage(`{}`))
	require.ErrorIs(t, err, tools.ErrGovernedDataAccessDenied)
	require.Equal(t, 1, calls)
}
