package service

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
)

// employeeAssistant resolves a platform-owned definition in the current tenant.
// Saved presets cannot change its tool authority or bind another tenant's provider.
func (s *customAgentService) employeeAssistant(ctx context.Context, tenantID uint64) (*types.CustomAgent, error) {
	agent := types.GetBuiltinAgentWithContext(ctx, types.BuiltinEmployeeAssistantID, tenantID)
	if agent == nil {
		return nil, fmt.Errorf("employee assistant definition is missing")
	}
	if s.scenarioCapabilities == nil {
		return nil, ErrAssistantScenarioCapabilityUnavailable
	}
	settings, err := s.scenarioCapabilities.ResolveAssistantScenarioCapabilities(ctx, tenantID)
	if err != nil || settings == nil {
		return nil, ErrAssistantScenarioCapabilityUnavailable
	}
	ready := false
	agent.WebSearchReady = &ready
	agent.Config.WebSearchEnabled = settings.Capabilities.ExternalSearch
	if agent.Config.WebSearchEnabled {
		if s.webSearchProviders == nil {
			return nil, ErrAssistantScenarioCapabilityUnavailable
		}
		provider, err := s.webSearchProviders.GetDefault(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		if provider != nil && provider.TenantID == tenantID && isValidProviderType(provider.Provider) &&
			validateProviderParameters(provider.Provider, provider.Parameters) == nil {
			agent.Config.WebSearchProviderID = provider.ID
			ready = true
		}
	}
	return agent, nil
}
