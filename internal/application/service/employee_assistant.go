package service

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
)

// employeeAssistant resolves a platform-owned definition for the current tenant.
// Saved presets cannot change its tool authority or select a different global provider.
func (s *customAgentService) employeeAssistant(ctx context.Context, tenantID uint64) (*types.CustomAgent, error) {
	agent, err := s.platformBuiltinAgent(ctx, types.BuiltinEmployeeAssistantID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("employee assistant definition is missing")
	}
	if s.scenarioCapabilities == nil {
		return nil, ErrAssistantScenarioCapabilityUnavailable
	}
	settings, err := s.scenarioCapabilities.ResolveAssistantScenarioCapabilities(ctx, tenantID)
	if err != nil || settings == nil {
		return nil, ErrAssistantScenarioCapabilityUnavailable
	}
	platformSkillsMode := agent.Config.SkillsSelectionMode
	if platformSkillsMode == "" {
		platformSkillsMode = "all"
	}
	agent.Config.SandboxConfigID = ""
	agent.Config.SelectedSkills = nil
	if platformSkillsMode != "none" && settings.Capabilities.Tools && s.sandboxDefault != nil {
		configID, err := s.sandboxDefault.DefaultSandboxConfigID(ctx)
		if err != nil {
			return nil, err
		}
		if configID != "" {
			agent.Config.SandboxConfigID = configID
			agent.Config.SkillsSelectionMode = platformSkillsMode
		}
	}
	if agent.Config.SandboxConfigID == "" {
		agent.Config.SkillsSelectionMode = "none"
	}
	ready := false
	agent.WebSearchReady = &ready
	agent.Config.WebSearchEnabled = agent.Config.WebSearchEnabled && settings.Capabilities.ExternalSearch
	if agent.Config.WebSearchEnabled {
		if s.webSearchProviders == nil {
			return nil, ErrAssistantScenarioCapabilityUnavailable
		}
		provider, err := s.webSearchProviders.GetDefault(ctx)
		if err != nil {
			return nil, err
		}
		if provider != nil && isValidProviderType(provider.Provider) &&
			validateProviderParameters(provider.Provider, provider.Parameters) == nil {
			agent.Config.WebSearchProviderID = provider.ID
			ready = true
		}
	}
	return agent, nil
}

// Employee sandbox authority comes from the canonical platform definition, never
// from a sandbox pinned by an older conversation after tools were disabled.
func employeeSandboxDisabled(config *types.AgentConfig) bool {
	return config != nil && config.EmployeeAssistant &&
		(!config.SkillsEnabled || config.SandboxConfigID == "")
}
