package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type assistantScenarioResolverStub struct {
	settings         *types.AssistantScenarioCapabilitySettings
	settingsByTenant map[uint64]*types.AssistantScenarioCapabilitySettings
	err              error
}

func (s assistantScenarioResolverStub) ResolveAssistantScenarioCapabilities(
	_ context.Context,
	tenantID uint64,
) (*types.AssistantScenarioCapabilitySettings, error) {
	if s.settingsByTenant != nil {
		return s.settingsByTenant[tenantID], s.err
	}
	return s.settings, s.err
}

type assistantScenarioMCPCatalogStub struct {
	services []*types.MCPService
	err      error
}

func (s assistantScenarioMCPCatalogStub) ListMCPServicesByIDs(
	context.Context,
	uint64,
	[]string,
) ([]*types.MCPService, error) {
	return s.services, s.err
}

func assistantScenarioSettings(externalSearch, mcp, tools bool) *types.AssistantScenarioCapabilitySettings {
	return &types.AssistantScenarioCapabilitySettings{
		ContractVersion: "AssistantScenarioCapabilityV1",
		Capabilities: types.AssistantScenarioCapabilities{
			ExternalSearch: externalSearch,
			MCP:            mcp,
			Tools:          tools,
		},
	}
}

func TestAssistantScenarioPolicyGatesSaveAndExecution(t *testing.T) {
	ctx := context.WithValue(context.Background(), types.SystemAdminContextKey, true)
	webAgent := &types.CustomAgent{Config: types.CustomAgentConfig{WebSearchEnabled: true}}

	require.ErrorIs(t, validateAssistantScenarioCapabilities(
		ctx, assistantScenarioResolverStub{settings: assistantScenarioSettings(false, false, false)}, 7, webAgent.Config, false,
	), ErrAssistantScenarioCapabilityDenied)
	require.NoError(t, validateAssistantScenarioExecution(
		ctx, assistantScenarioResolverStub{settings: assistantScenarioSettings(false, false, false)}, nil, 7, webAgent, &types.QARequest{},
	))
	require.ErrorIs(t, validateAssistantScenarioExecution(
		ctx, assistantScenarioResolverStub{settings: assistantScenarioSettings(false, false, false)}, nil, 7, webAgent, &types.QARequest{WebSearchEnabled: true},
	), ErrAssistantScenarioCapabilityDenied)
	require.ErrorIs(t, validateAssistantScenarioCapabilities(
		ctx, assistantScenarioResolverStub{err: interfaces.ErrAICapabilityUnavailable}, 7, webAgent.Config, false,
	), ErrAssistantScenarioCapabilityUnavailable)

	selectedMCPAgent := &types.CustomAgent{Config: types.CustomAgentConfig{
		MCPSelectionMode: "selected",
		MCPServices:      []string{"approved"},
	}}
	enabledMCP := assistantScenarioMCPCatalogStub{services: []*types.MCPService{{ID: "approved", Enabled: true}}}
	require.ErrorIs(t, validateAssistantScenarioExecution(
		ctx,
		assistantScenarioResolverStub{settings: assistantScenarioSettings(false, true, false)},
		enabledMCP,
		7,
		selectedMCPAgent,
		&types.QARequest{MCPServiceIDs: []string{"unapproved"}},
	), ErrAssistantScenarioCapabilityDenied)
	require.NoError(t, validateAssistantScenarioExecution(
		ctx,
		assistantScenarioResolverStub{settings: assistantScenarioSettings(false, true, false)},
		enabledMCP,
		7,
		selectedMCPAgent,
		&types.QARequest{MCPServiceIDs: []string{"approved"}},
	))

	require.ErrorIs(t, validateAssistantScenarioMCPSelection(
		ctx,
		assistantScenarioMCPCatalogStub{services: []*types.MCPService{{ID: "approved", Enabled: false}}},
		7,
		selectedMCPAgent.Config,
	), ErrAssistantScenarioConfigurationInvalid)

	sharedCtx := context.WithValue(ctx, types.UserIDContextKey, "human-1")
	sharedCtx = types.WithAuthorizedSharedAgentExecution(sharedCtx, 10, 20, "shared-agent")
	sharedAgent := &types.CustomAgent{ID: "shared-agent", TenantID: 20, Config: types.CustomAgentConfig{WebSearchEnabled: true}}
	require.ErrorIs(t, validateAssistantScenarioExecution(
		sharedCtx,
		assistantScenarioResolverStub{settingsByTenant: map[uint64]*types.AssistantScenarioCapabilitySettings{
			10: assistantScenarioSettings(false, false, false),
			20: assistantScenarioSettings(true, false, false),
		}},
		nil,
		20,
		sharedAgent,
		&types.QARequest{SharedAgentReadOnly: true, WebSearchEnabled: true},
	), ErrAssistantScenarioCapabilityDenied)
}
