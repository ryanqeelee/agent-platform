package service

import (
	"context"
	"fmt"
	"github.com/Tencent/WeKnora/internal/types"
)

// Native analysis has independent platform-owned prompts and tools. It shares
// the existing tenant sandbox configuration, with existing sandbox/Skills/network capability switches.
func (s *customAgentService) nativeAnalysisAgent(ctx context.Context, id string, tenantID uint64) (*types.CustomAgent, error) {
	agent, err := s.platformBuiltinAgent(ctx, id, tenantID)
	if err != nil {
		return nil, fmt.Errorf("native analysis definition is missing")
	}
	if s.scenarioCapabilities == nil {
		return nil, ErrAssistantScenarioCapabilityUnavailable
	}
	settings, err := s.scenarioCapabilities.ResolveAssistantScenarioCapabilities(ctx, tenantID)
	if err != nil || settings == nil {
		return nil, ErrAssistantScenarioCapabilityUnavailable
	}
	agent.Config.SandboxConfigID = ""
	platformSkillsMode := agent.Config.SkillsSelectionMode
	if platformSkillsMode != "none" && settings.Capabilities.Tools && s.sandboxConfigs != nil {
		if s.provisionEmployeeSandbox != nil {
			if err := s.provisionEmployeeSandbox(ctx, tenantID); err != nil {
				return nil, err
			}
		}
		configs, err := s.sandboxConfigs.ListByTenant(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		for _, config := range configs {
			if config.Name != "employee-assistant" {
				continue
			}
			if config.TenantID != tenantID || agent.Config.SandboxConfigID != "" {
				return nil, fmt.Errorf("analysis sandbox configuration is ambiguous or outside workspace")
			}
			agent.Config.SandboxConfigID = config.ID
		}
	}
	if agent.Config.SandboxConfigID == "" {
		agent.Config.SkillsSelectionMode = "none"
		agent.Config.SelectedSkills = nil
	}
	agent.Config.WebSearchEnabled = agent.Config.WebSearchEnabled && settings.Capabilities.ExternalSearch
	ready := false
	agent.WebSearchReady = &ready
	if agent.Config.WebSearchEnabled && s.webSearchProviders != nil {
		provider, err := s.webSearchProviders.GetDefault(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		if provider != nil && provider.TenantID == tenantID && isValidProviderType(provider.Provider) && validateProviderParameters(provider.Provider, provider.Parameters) == nil {
			agent.Config.WebSearchProviderID = provider.ID
			ready = true
		}
	}
	return agent, nil
}
