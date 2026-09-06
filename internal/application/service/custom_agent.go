package service

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

// Custom agent related errors
var (
	ErrAgentNotFound                          = errors.New("agent not found")
	ErrCannotModifyBuiltin                    = errors.New("cannot modify built-in agent basic info")
	ErrCannotDeleteBuiltin                    = errors.New("cannot delete built-in agent")
	ErrAgentNameRequired                      = errors.New("agent name is required")
	ErrAssistantScenarioCapabilityDenied      = errors.New("该助理场景能力未由平台启用，请联系平台管理员")
	ErrAssistantScenarioCapabilityUnavailable = errors.New("助理场景能力暂不可用，请稍后重试")
	ErrAssistantScenarioConfigurationInvalid  = errors.New("助理场景包含已停用或不存在的服务，请重新选择后保存")
)

const (
	// suggestionDefaultLimit is the fallback count when neither the caller nor
	// the agent configuration specifies how many suggestions to return.
	suggestionDefaultLimit = 6
	// suggestionMaxLimit caps how many suggestions a single request may ask for.
	// It bounds the downstream candidate pool (limit*5) so an oversized limit
	// cannot turn into an unbounded chunk query.
	suggestionMaxLimit = 30
)

// AgentView exposes raw platform bindings only to SystemAdmin. Every workspace
// principal receives the same enterprise-safe Agent projection.
func AgentView(ctx context.Context, agent *types.CustomAgent) *types.CustomAgent {
	if agent == nil {
		return nil
	}
	if types.IsSystemAdminFromContext(ctx) {
		return agent
	}
	view := *agent
	view.Config = types.CustomAgentConfig{}
	applyWorkspaceAgentConfig(&view.Config, agent.Config)
	if types.TenantRoleFromContext(ctx).HasPermission(types.TenantRoleAdmin) {
		applyEnterpriseScenarioConfig(&view.Config, agent.Config)
		if view.Config.MCPSelectionMode == "all" {
			view.Config.MCPSelectionMode = "none"
			view.Config.MCPServices = nil
		}
	}
	return &view
}

// applyWorkspaceAgentConfig is the enterprise-authorable Agent contract. New
// runtime fields stay platform-only by default until deliberately added here.
func applyWorkspaceAgentConfig(dst *types.CustomAgentConfig, src types.CustomAgentConfig) {
	dst.AgentMode = src.AgentMode
	dst.AgentType = src.AgentType
	dst.SystemPrompt = src.SystemPrompt
	dst.ContextTemplate = src.ContextTemplate
	dst.CitationEnabled = src.CitationEnabled
	dst.KBSelectionMode = src.KBSelectionMode
	dst.KnowledgeBases = src.KnowledgeBases
	dst.RetrieveKBOnlyWhenMentioned = src.RetrieveKBOnlyWhenMentioned
	dst.SupportedFileTypes = src.SupportedFileTypes
	// Capability booleans are safe workspace-facing UI contracts. Provider/model/storage
	// bindings remain platform-only, but hiding these flags made enabled features look absent.
	dst.WebSearchEnabled = src.WebSearchEnabled
	dst.ImageUploadEnabled = src.ImageUploadEnabled
	dst.FallbackStrategy = src.FallbackStrategy
	dst.FallbackResponse = src.FallbackResponse
	dst.FallbackPrompt = src.FallbackPrompt
	dst.IntentPrompts = src.IntentPrompts
	dst.QuestionSuggestions = nil
	if src.QuestionSuggestions != nil {
		copy := *src.QuestionSuggestions
		copy.FollowUps.ModelID = ""
		dst.QuestionSuggestions = &copy
	}
}

func applyEnterpriseScenarioConfig(dst *types.CustomAgentConfig, src types.CustomAgentConfig) {
	dst.AllowedTools = src.AllowedTools
	dst.MCPSelectionMode = src.MCPSelectionMode
	dst.MCPServices = src.MCPServices
}

func preserveAgentPlatformBindings(next *types.CustomAgentConfig, current types.CustomAgentConfig) {
	business := types.CustomAgentConfig{}
	applyWorkspaceAgentConfig(&business, *next)
	applyEnterpriseScenarioConfig(&business, *next)
	*next = current
	currentModelID := ""
	if current.QuestionSuggestions != nil {
		currentModelID = current.QuestionSuggestions.FollowUps.ModelID
	}
	applyWorkspaceAgentConfig(next, business)
	applyEnterpriseScenarioConfig(next, business)
	if next.QuestionSuggestions != nil && current.QuestionSuggestions != nil {
		next.QuestionSuggestions.FollowUps.ModelID = currentModelID
	}
}

// customAgentService implements the CustomAgentService interface
type customAgentService struct {
	repo                 interfaces.CustomAgentRepository
	chunkRepo            interfaces.ChunkRepository
	kbService            interfaces.KnowledgeBaseService
	kbShareService       interfaces.KBShareService
	wikiPageRepo         interfaces.WikiPageRepository
	tagRepo              interfaces.KnowledgeTagRepository
	knowledgeRepo        interfaces.KnowledgeRepository
	scenarioCapabilities interfaces.AssistantScenarioCapabilityResolver
	mcpServices          interfaces.MCPServiceService
	webSearchProviders   interfaces.WebSearchProviderRepository
}

// NewCustomAgentService creates a new custom agent service
func NewCustomAgentService(
	repo interfaces.CustomAgentRepository,
	chunkRepo interfaces.ChunkRepository,
	kbService interfaces.KnowledgeBaseService,
	kbShareService interfaces.KBShareService,
	wikiPageRepo interfaces.WikiPageRepository,
	tagRepo interfaces.KnowledgeTagRepository,
	knowledgeRepo interfaces.KnowledgeRepository,
	scenarioCapabilities interfaces.AssistantScenarioCapabilityResolver,
	mcpServices interfaces.MCPServiceService,
	webSearchProviders interfaces.WebSearchProviderRepository,
) interfaces.CustomAgentService {
	return &customAgentService{
		repo:                 repo,
		chunkRepo:            chunkRepo,
		kbService:            kbService,
		kbShareService:       kbShareService,
		wikiPageRepo:         wikiPageRepo,
		tagRepo:              tagRepo,
		knowledgeRepo:        knowledgeRepo,
		scenarioCapabilities: scenarioCapabilities,
		mcpServices:          mcpServices,
		webSearchProviders:   webSearchProviders,
	}
}

type assistantScenarioMCPCatalog interface {
	ListMCPServicesByIDs(context.Context, uint64, []string) ([]*types.MCPService, error)
}

func assistantScenarioCapabilityUse(
	config types.CustomAgentConfig,
	allowMCPAll bool,
) (types.AssistantScenarioCapabilities, error) {
	use := types.AssistantScenarioCapabilities{
		ExternalSearch: config.WebSearchEnabled,
	}
	// Basic read tools belong to employee assistance, not optional execution extensions.
	allowed := config.AllowedTools
	if config.AgentMode == types.AgentModeSmartReasoning && len(allowed) == 0 {
		allowed = tools.DefaultAllowedTools()
	}
	for _, name := range allowed {
		if name == tools.ToolWebSearch || name == tools.ToolWebFetch {
			use.ExternalSearch = true
		} else if !employeeAssistantReadTool(name) {
			use.Tools = true
		}
	}
	if config.SandboxConfigID != "" || config.SkillsSelectionMode == "all" ||
		config.SkillsSelectionMode == "selected" && len(config.SelectedSkills) > 0 {
		use.Tools = true
	}
	switch config.MCPSelectionMode {
	case "", "none":
	case "selected":
		use.MCP = len(config.MCPServices) > 0
	case "all":
		if !allowMCPAll {
			return use, ErrAssistantScenarioCapabilityDenied
		}
		use.MCP = true
	default:
		return use, ErrAssistantScenarioCapabilityDenied
	}
	return use, nil
}

func validateAssistantScenarioCapabilities(
	ctx context.Context,
	resolver interfaces.AssistantScenarioCapabilityResolver,
	tenantID uint64,
	config types.CustomAgentConfig,
	allowMCPAll bool,
) error {
	use, err := assistantScenarioCapabilityUse(config, allowMCPAll)
	if err != nil {
		return err
	}
	if !use.ExternalSearch && !use.MCP && !use.Tools {
		return nil
	}
	if resolver == nil {
		return ErrAssistantScenarioCapabilityUnavailable
	}
	settings, err := resolver.ResolveAssistantScenarioCapabilities(ctx, tenantID)
	if err != nil || settings == nil {
		return ErrAssistantScenarioCapabilityUnavailable
	}
	if use.ExternalSearch && !settings.Capabilities.ExternalSearch ||
		use.MCP && !settings.Capabilities.MCP ||
		use.Tools && !settings.Capabilities.Tools {
		return ErrAssistantScenarioCapabilityDenied
	}
	return nil
}

func validateAssistantScenarioMCPSelection(
	ctx context.Context,
	catalog assistantScenarioMCPCatalog,
	tenantID uint64,
	config types.CustomAgentConfig,
) error {
	if config.MCPSelectionMode != "selected" || len(config.MCPServices) == 0 {
		return nil
	}
	if catalog == nil {
		return ErrAssistantScenarioCapabilityUnavailable
	}
	services, err := catalog.ListMCPServicesByIDs(ctx, tenantID, config.MCPServices)
	if err != nil {
		return ErrAssistantScenarioCapabilityUnavailable
	}
	enabled := make(map[string]struct{}, len(services))
	for _, service := range services {
		if service != nil && service.Enabled {
			enabled[service.ID] = struct{}{}
		}
	}
	for _, id := range config.MCPServices {
		if _, ok := enabled[id]; !ok {
			return ErrAssistantScenarioConfigurationInvalid
		}
	}
	return nil
}

func validateAssistantScenarioExecution(
	ctx context.Context,
	resolver interfaces.AssistantScenarioCapabilityResolver,
	mcpCatalog assistantScenarioMCPCatalog,
	tenantID uint64,
	agent *types.CustomAgent,
	req *types.QARequest,
) error {
	if agent == nil {
		if req.WebSearchEnabled || len(req.MCPServiceIDs) > 0 {
			return ErrAssistantScenarioCapabilityDenied
		}
		return nil
	}
	// Execution is gated by the capabilities this request will actually use. An agent may be
	// configured to offer optional web search while the current request keeps it off; treating
	// that dormant option as active makes ordinary knowledge Q&A unavailable for plans without
	// external search. Authoring still validates the full saved configuration above.
	executionConfig := agent.Config
	executionConfig.WebSearchEnabled = req.WebSearchEnabled && agent.Config.WebSearchEnabled
	if err := validateAssistantScenarioCapabilities(ctx, resolver, tenantID, executionConfig, true); err != nil {
		return err
	}
	if req.SharedAgentReadOnly {
		callerTenantID, sourceTenantID, agentID, ok := types.AuthorizedSharedAgentExecutionFromContext(ctx)
		if !ok || sourceTenantID != tenantID || agentID != agent.ID {
			return ErrAssistantScenarioCapabilityDenied
		}
		if err := validateAssistantScenarioCapabilities(ctx, resolver, callerTenantID, executionConfig, true); err != nil {
			return err
		}
	}
	if err := validateAssistantScenarioMCPSelection(ctx, mcpCatalog, tenantID, agent.Config); err != nil {
		return err
	}
	if req.WebSearchEnabled && !agent.Config.WebSearchEnabled {
		return ErrAssistantScenarioCapabilityDenied
	}
	if len(req.MCPServiceIDs) == 0 {
		return nil
	}
	switch agent.Config.MCPSelectionMode {
	case "all":
		return nil
	case "selected":
		allowed := make(map[string]struct{}, len(agent.Config.MCPServices))
		for _, id := range agent.Config.MCPServices {
			allowed[id] = struct{}{}
		}
		for _, id := range req.MCPServiceIDs {
			if _, ok := allowed[id]; !ok {
				return ErrAssistantScenarioCapabilityDenied
			}
		}
		return nil
	default:
		return ErrAssistantScenarioCapabilityDenied
	}
}

func (s *customAgentService) GetAssistantScenarioCapabilities(
	ctx context.Context,
) (*types.AssistantScenarioCapabilitySettings, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || s.scenarioCapabilities == nil {
		return nil, ErrAssistantScenarioCapabilityUnavailable
	}
	settings, err := s.scenarioCapabilities.ResolveAssistantScenarioCapabilities(ctx, tenantID)
	if err != nil || settings == nil {
		return nil, ErrAssistantScenarioCapabilityUnavailable
	}
	return settings, nil
}

func (s *customAgentService) validateAssistantScenarioConfig(
	ctx context.Context,
	tenantID uint64,
	config types.CustomAgentConfig,
) error {
	if !types.IsSystemAdminFromContext(ctx) {
		if err := validateAssistantScenarioCapabilities(
			ctx,
			s.scenarioCapabilities,
			tenantID,
			config,
			false,
		); err != nil {
			return err
		}
	}
	return validateAssistantScenarioMCPSelection(ctx, s.mcpServices, tenantID, config)
}

// CreateAgent creates a new custom agent
func (s *customAgentService) CreateAgent(ctx context.Context, agent *types.CustomAgent) (*types.CustomAgent, error) {
	// Validate required fields
	if strings.TrimSpace(agent.Name) == "" {
		return nil, ErrAgentNameRequired
	}

	// Generate UUID and set creation timestamps
	if agent.ID == "" {
		agent.ID = uuid.New().String()
	}

	// Get tenant ID from context
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrInvalidTenantID
	}
	agent.TenantID = tenantID

	// Record the creator for display and audit. Agent authoring itself is
	// Admin+; ownership no longer grants a Contributor mutation authority.
	if uid, ok := types.UserIDFromContext(ctx); ok && !types.IsSyntheticUserID(uid) {
		agent.CreatedBy = uid
	}

	// Set timestamps
	agent.CreatedAt = time.Now()
	agent.UpdatedAt = time.Now()

	// Ensure agent mode is set for user-created agents
	if agent.Config.AgentMode == "" {
		agent.Config.AgentMode = types.AgentModeQuickAnswer
	}

	// Cannot create built-in agents
	agent.IsBuiltin = false
	if !types.IsSystemAdminFromContext(ctx) {
		business := agent.Config
		agent.Config = types.CustomAgentConfig{}
		applyWorkspaceAgentConfig(&agent.Config, business)
		applyEnterpriseScenarioConfig(&agent.Config, business)
	}

	// Set defaults
	agent.EnsureDefaults()
	if err := agent.Config.QuestionSuggestions.Validate(); err != nil {
		return nil, err
	}
	if err := s.validateAssistantScenarioConfig(ctx, tenantID, agent.Config); err != nil {
		return nil, err
	}

	logger.Infof(ctx, "Creating custom agent, ID: %s, tenant ID: %d, name: %s, agent_mode: %s",
		agent.ID, agent.TenantID, agent.Name, agent.Config.AgentMode)

	if err := s.repo.CreateAgent(ctx, agent); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id":  agent.ID,
			"tenant_id": agent.TenantID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Custom agent created successfully, ID: %s, name: %s", agent.ID, agent.Name)
	return agent, nil
}

// GetAgentByID retrieves an agent by its ID (including built-in agents)
func (s *customAgentService) GetAgentByID(ctx context.Context, id string) (*types.CustomAgent, error) {
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		return nil, errors.New("agent ID cannot be empty")
	}

	// Get tenant ID from context
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrInvalidTenantID
	}
	if id == types.BuiltinEmployeeAssistantID {
		return s.employeeAssistant(ctx, tenantID)
	}

	// Check if it's a built-in agent using the registry
	if types.IsBuiltinAgentID(id) {
		// Try to get from database first (for customized config)
		agent, err := s.repo.GetAgentByID(ctx, id, tenantID)
		if err == nil {
			// Found in database, overlay locale-specific name/description/avatar
			agent.EnsureDefaults()
			types.ApplyBuiltinAgentLocalization(ctx, agent)
			return agent, nil
		}
		// Not in database, return default built-in agent from registry (i18n-aware)
		if builtinAgent := types.GetBuiltinAgentWithContext(ctx, id, tenantID); builtinAgent != nil {
			return builtinAgent, nil
		}
	}

	// Query from database
	agent, err := s.repo.GetAgentByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		return nil, err
	}

	agent.EnsureDefaults()
	return agent, nil
}

// GetAgentByIDAndTenant retrieves an agent by ID and tenant (for shared agents; does not resolve built-in)
func (s *customAgentService) GetAgentByIDAndTenant(ctx context.Context, id string, tenantID uint64) (*types.CustomAgent, error) {
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		return nil, errors.New("agent ID cannot be empty")
	}
	agent, err := s.repo.GetAgentByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}
	agent.EnsureDefaults()
	return agent, nil
}

// ListAgents lists all agents for the current tenant (including built-in agents)
func (s *customAgentService) ListAgents(ctx context.Context) ([]*types.CustomAgent, error) {
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrInvalidTenantID
	}

	// Get all agents from database (including built-in agents with customized config)
	allAgents, err := s.repo.ListAgentsByTenantID(ctx, tenantID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": tenantID,
		})
		return nil, err
	}

	// Track which built-in agents exist in database
	builtinInDB := make(map[string]bool)
	for _, agent := range allAgents {
		agent.EnsureDefaults()
		if types.IsBuiltinAgentID(agent.ID) {
			builtinInDB[agent.ID] = true
		}
	}

	// Build result: built-in agents first, then custom agents
	builtinIDs := types.GetBuiltinAgentIDs()
	result := make([]*types.CustomAgent, 0, len(allAgents)+len(builtinIDs))

	// Add built-in agents in order
	for _, builtinID := range builtinIDs {
		if builtinID == types.BuiltinEmployeeAssistantID {
			agent, err := s.employeeAssistant(ctx, tenantID)
			if err != nil {
				return nil, err
			}
			result = append(result, agent)
			continue
		}
		if builtinInDB[builtinID] {
			// Use customized config from database
			for _, agent := range allAgents {
				if agent.ID == builtinID {
					types.ApplyBuiltinAgentLocalization(ctx, agent)
					result = append(result, agent)
					break
				}
			}
		} else {
			// Use default built-in agent (i18n-aware)
			if agent := types.GetBuiltinAgentWithContext(ctx, builtinID, tenantID); agent != nil {
				result = append(result, agent)
			}
		}
	}

	// Add custom agents
	for _, agent := range allAgents {
		if !types.IsBuiltinAgentID(agent.ID) {
			result = append(result, agent)
		}
	}

	return result, nil
}

// UpdateAgent updates an agent's information
func (s *customAgentService) UpdateAgent(ctx context.Context, agent *types.CustomAgent) (*types.CustomAgent, error) {
	if agent.ID == types.BuiltinEmployeeAssistantID {
		return nil, ErrCannotModifyBuiltin
	}
	if agent.ID == "" {
		logger.Error(ctx, "Agent ID is empty")
		return nil, errors.New("agent ID cannot be empty")
	}

	// Get tenant ID from context
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrInvalidTenantID
	}

	// Handle built-in agents specially using registry
	if types.IsBuiltinAgentID(agent.ID) {
		return s.updateBuiltinAgent(ctx, agent, tenantID)
	}

	// Get existing agent
	existingAgent, err := s.repo.GetAgentByID(ctx, agent.ID, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		return nil, err
	}

	// Cannot modify built-in status
	if existingAgent.IsBuiltin {
		return nil, ErrCannotModifyBuiltin
	}

	// Validate name
	if strings.TrimSpace(agent.Name) == "" {
		return nil, ErrAgentNameRequired
	}

	// Update fields
	existingAgent.Name = agent.Name
	existingAgent.Description = agent.Description
	existingAgent.Avatar = agent.Avatar
	if !types.IsSystemAdminFromContext(ctx) {
		preserveAgentPlatformBindings(&agent.Config, existingAgent.Config)
	}
	existingAgent.Config = agent.Config
	existingAgent.UpdatedAt = time.Now()

	// Ensure defaults
	existingAgent.EnsureDefaults()
	if err := existingAgent.Config.QuestionSuggestions.Validate(); err != nil {
		return nil, err
	}
	if err := s.validateAssistantScenarioConfig(ctx, tenantID, existingAgent.Config); err != nil {
		return nil, err
	}

	logger.Infof(ctx, "Updating custom agent, ID: %s, name: %s", agent.ID, agent.Name)

	if err := s.repo.UpdateAgent(ctx, existingAgent); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": agent.ID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Custom agent updated successfully, ID: %s", agent.ID)
	return existingAgent, nil
}

// updateBuiltinAgent updates a built-in agent's configuration (but not basic info)
func (s *customAgentService) updateBuiltinAgent(ctx context.Context, agent *types.CustomAgent, tenantID uint64) (*types.CustomAgent, error) {
	// Persist locale-independent display fields (the YAML "default" locale) so
	// read paths that skip builtin localization see a stable language.
	defaultAgent := types.GetBuiltinAgent(agent.ID, tenantID)
	if defaultAgent == nil {
		return nil, ErrAgentNotFound
	}

	// Try to get existing customized config from database
	existingAgent, err := s.repo.GetAgentByID(ctx, agent.ID, tenantID)
	if err != nil && !errors.Is(err, repository.ErrCustomAgentNotFound) {
		return nil, err
	}

	if existingAgent != nil {
		// Update existing record - only update config, keep basic info unchanged
		if !types.IsSystemAdminFromContext(ctx) {
			preserveAgentPlatformBindings(&agent.Config, existingAgent.Config)
		}
		existingAgent.Config = agent.Config
		existingAgent.UpdatedAt = time.Now()
		existingAgent.EnsureDefaults()
		if err := existingAgent.Config.QuestionSuggestions.Validate(); err != nil {
			return nil, err
		}
		if err := s.validateAssistantScenarioConfig(ctx, tenantID, existingAgent.Config); err != nil {
			return nil, err
		}

		logger.Infof(ctx, "Updating built-in agent config, ID: %s", agent.ID)

		if err := s.repo.UpdateAgent(ctx, existingAgent); err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"agent_id": agent.ID,
			})
			return nil, err
		}

		logger.Infof(ctx, "Built-in agent config updated successfully, ID: %s", agent.ID)
		types.ApplyBuiltinAgentLocalization(ctx, existingAgent)
		return existingAgent, nil
	}

	// Create new record for built-in agent with customized config
	if !types.IsSystemAdminFromContext(ctx) {
		preserveAgentPlatformBindings(&agent.Config, defaultAgent.Config)
	}
	newAgent := &types.CustomAgent{
		ID:          defaultAgent.ID,
		Name:        defaultAgent.Name,
		Description: defaultAgent.Description,
		Avatar:      defaultAgent.Avatar,
		IsBuiltin:   true,
		TenantID:    tenantID,
		Config:      agent.Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	newAgent.EnsureDefaults()
	if err := newAgent.Config.QuestionSuggestions.Validate(); err != nil {
		return nil, err
	}
	if err := s.validateAssistantScenarioConfig(ctx, tenantID, newAgent.Config); err != nil {
		return nil, err
	}

	logger.Infof(ctx, "Creating built-in agent config record, ID: %s, tenant ID: %d", agent.ID, tenantID)

	if err := s.repo.CreateAgent(ctx, newAgent); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id":  agent.ID,
			"tenant_id": tenantID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Built-in agent config record created successfully, ID: %s", agent.ID)
	types.ApplyBuiltinAgentLocalization(ctx, newAgent)
	return newAgent, nil
}

// DeleteAgent deletes an agent
func (s *customAgentService) DeleteAgent(ctx context.Context, id string) error {
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		return errors.New("agent ID cannot be empty")
	}

	// Cannot delete built-in agents using registry check
	if types.IsBuiltinAgentID(id) {
		return ErrCannotDeleteBuiltin
	}

	// Get tenant ID from context
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return ErrInvalidTenantID
	}

	// Get existing agent to verify ownership
	existingAgent, err := s.repo.GetAgentByID(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, repository.ErrCustomAgentNotFound) {
			return ErrAgentNotFound
		}
		return err
	}

	// Cannot delete built-in agents
	if existingAgent.IsBuiltin {
		return ErrCannotDeleteBuiltin
	}

	logger.Infof(ctx, "Deleting custom agent, ID: %s", id)

	if err := s.repo.DeleteAgent(ctx, id, tenantID); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"agent_id": id,
		})
		return err
	}

	logger.Infof(ctx, "Custom agent deleted successfully, ID: %s", id)
	return nil
}

// CopyAgent creates a copy of an existing agent
func (s *customAgentService) CopyAgent(ctx context.Context, id string) (*types.CustomAgent, error) {
	if id == "" {
		logger.Error(ctx, "Agent ID is empty")
		return nil, errors.New("agent ID cannot be empty")
	}

	// Get tenant ID from context
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrInvalidTenantID
	}

	// Get the source agent
	sourceAgent, err := s.GetAgentByID(ctx, id)
	if err != nil {
		return nil, err
	}

	// Create a new agent with copied data
	newAgent := &types.CustomAgent{
		ID:          uuid.New().String(),
		Name:        sourceAgent.Name + " (副本)",
		Description: sourceAgent.Description,
		Avatar:      sourceAgent.Avatar,
		IsBuiltin:   false, // Copied agents are never built-in
		TenantID:    tenantID,
		Config:      sourceAgent.Config,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	// The clone is owned by whoever ran the copy, not the original
	// creator — same reasoning as CopyKnowledgeBase. Skip synthetic
	// API-key users.
	if uid, ok := types.UserIDFromContext(ctx); ok && !types.IsSyntheticUserID(uid) {
		newAgent.CreatedBy = uid
	}

	// Ensure defaults
	newAgent.EnsureDefaults()
	if err := s.validateAssistantScenarioConfig(ctx, tenantID, newAgent.Config); err != nil {
		return nil, err
	}

	logger.Infof(ctx, "Copying agent, source ID: %s, new ID: %s", id, newAgent.ID)

	if err := s.repo.CreateAgent(ctx, newAgent); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"source_agent_id": id,
			"new_agent_id":    newAgent.ID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Agent copied successfully, source ID: %s, new ID: %s", id, newAgent.ID)
	return newAgent, nil
}

// GetSuggestedQuestions returns suggested questions for the agent based on its
// associated knowledge bases.
func (s *customAgentService) GetSuggestedQuestions(
	ctx context.Context,
	agentID string,
	kbIDs []string,
	knowledgeIDs []string,
	tagScopes []types.TagScope,
	limit int,
) ([]types.SuggestedQuestion, error) {
	return s.getSuggestedQuestions(ctx, agentID, kbIDs, knowledgeIDs, tagScopes, limit, true)
}

func (s *customAgentService) GetKnowledgeSuggestedQuestions(
	ctx context.Context,
	agentID string,
	kbIDs []string,
	knowledgeIDs []string,
	tagScopes []types.TagScope,
	limit int,
) ([]types.SuggestedQuestion, error) {
	return s.getSuggestedQuestions(ctx, agentID, kbIDs, knowledgeIDs, tagScopes, limit, false)
}

func (s *customAgentService) getSuggestedQuestions(
	ctx context.Context,
	agentID string,
	kbIDs []string,
	knowledgeIDs []string,
	tagScopes []types.TagScope,
	limit int,
	includeCurated bool,
) ([]types.SuggestedQuestion, error) {
	// A non-positive limit means "unspecified": fall back to the agent's
	// configured Starters.Count (below) or the default. An explicit limit is
	// authoritative and only bounded by the safety cap.
	limitProvided := limit > 0
	if !limitProvided {
		limit = suggestionDefaultLimit
	}
	if limit > suggestionMaxLimit {
		limit = suggestionMaxLimit
	}

	if err := types.AuthorizeTenantAPIKeyKnowledgeTargets(ctx, kbIDs, knowledgeIDs); err != nil {
		return nil, err
	}
	scopeTagIDs := flattenTagScopeIDs(tagScopes)
	if err := types.AuthorizeTenantAPIKeyOptionalTagIDs(ctx, scopeTagIDs); err != nil {
		return nil, err
	}

	// Get tenant ID from context
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil, ErrInvalidTenantID
	}

	// Get agent configuration
	agent, err := s.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, err
	}

	var curated []types.SuggestedQuestion
	starterMode := types.SuggestionModeKnowledge

	if includeCurated {
		suggestionConfig := agent.Config.QuestionSuggestions
		if suggestionConfig == nil || !suggestionConfig.Starters.Enabled {
			return []types.SuggestedQuestion{}, nil
		}
		// Starters.Count is the agent-author default, applied only when the
		// caller did not request a specific limit. An explicit limit stays
		// authoritative so ?limit=N actually changes how many are returned.
		if !limitProvided && suggestionConfig.Starters.Count > 0 {
			limit = suggestionConfig.Starters.Count
		}
		starterMode = suggestionConfig.Starters.Mode
		// Add curated agent prompts first (highest priority).
		if suggestionConfig.Starters.Mode == types.SuggestionModeCurated ||
			suggestionConfig.Starters.Mode == types.SuggestionModeHybrid {
			for _, prompt := range suggestionConfig.Starters.Items {
				if strings.TrimSpace(prompt) == "" {
					continue
				}
				curated = append(curated, types.SuggestedQuestion{
					Question: prompt,
					Source:   "agent_config",
				})
			}
		}
		if suggestionConfig.Starters.Mode == types.SuggestionModeCurated {
			return s.truncateQuestions(curated, limit), nil
		}
	}

	resolvedTags := resolvedSuggestionTagScopes{}
	if len(scopeTagIDs) > 0 {
		var err error
		resolvedTags, err = s.resolveSuggestionTagScopes(ctx, tenantID, tagScopes)
		if err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"agent_id":      agentID,
				"scope_tag_ids": scopeTagIDs,
			})
			return finalizeStarterSuggestions(curated, nil, starterMode, limit), nil
		}
		knowledgeIDs = mergeUniqueStrings(knowledgeIDs, resolvedTags.KnowledgeIDs)
		if len(knowledgeIDs) == 0 && len(resolvedTags.TagIDsByTenant) == 0 {
			return finalizeStarterSuggestions(curated, nil, starterMode, limit), nil
		}
	}

	// Explicit document IDs must inherit their KB's current authorization.
	// The chunk queries below accept document IDs directly, so validating only
	// kbIDs would let a revoked same-tenant Business Role grant be bypassed.
	if len(knowledgeIDs) > 0 {
		if s.knowledgeRepo == nil {
			knowledgeIDs = nil
		} else {
			knowledgeByID := make(map[string]*types.Knowledge, len(knowledgeIDs))
			var parentKBIDs []string
			for _, knowledgeID := range knowledgeIDs {
				knowledge, err := s.knowledgeRepo.GetKnowledgeByIDOnly(ctx, knowledgeID)
				if err != nil || knowledge == nil || knowledge.KnowledgeBaseID == "" {
					continue
				}
				knowledgeByID[knowledgeID] = knowledge
				parentKBIDs = mergeUniqueStrings(parentKBIDs, []string{knowledge.KnowledgeBaseID})
			}
			allowedKBs := make(map[string]bool, len(parentKBIDs))
			for _, group := range s.groupKBIDsByEffectiveTenant(ctx, tenantID, parentKBIDs) {
				for _, kbID := range group {
					allowedKBs[kbID] = true
				}
			}
			filtered := make([]string, 0, len(knowledgeIDs))
			for _, knowledgeID := range knowledgeIDs {
				if knowledge := knowledgeByID[knowledgeID]; knowledge != nil && allowedKBs[knowledge.KnowledgeBaseID] {
					filtered = append(filtered, knowledgeID)
				}
			}
			knowledgeIDs = filtered
		}
	}

	// 2. Determine knowledge base scope
	effectiveKBIDs := kbIDs
	if len(effectiveKBIDs) == 0 && len(knowledgeIDs) == 0 && len(resolvedTags.TagIDsByTenant) == 0 {
		// Use agent's KB configuration
		switch agent.Config.KBSelectionMode {
		case "all":
			kbs, err := s.kbService.ListKnowledgeBases(ctx)
			if err != nil {
				logger.ErrorWithFields(ctx, err, map[string]interface{}{
					"agent_id": agentID,
				})
				// Return what we have so far (agent_config suggestions)
				return finalizeStarterSuggestions(curated, nil, starterMode, limit), nil
			}
			// Honor the agent's implicit/explicit capability requirements so
			// e.g. a quick-answer (RAG-only) agent doesn't surface wiki-only
			// KBs whose wiki pages it could never answer from. Same filter
			// the @ mention dropdown applies on the frontend.
			capFilter := tools.DeriveKBFilterForAgent(agent.Config.AgentMode, agent.Config.AllowedTools)
			for _, kb := range kbs {
				if !capFilter.IsEmpty() &&
					!tools.KBSatisfiesAgentRequirements(kb.Capabilities(), agent.Config.AgentMode, agent.Config.AllowedTools) {
					continue
				}
				effectiveKBIDs = append(effectiveKBIDs, kb.ID)
			}
		case "selected":
			effectiveKBIDs = agent.Config.KnowledgeBases
		case "none":
			// No KB access, return agent_config suggestions only
			return finalizeStarterSuggestions(curated, nil, starterMode, limit), nil
		default:
			// Default to agent's configured KBs
			effectiveKBIDs = agent.Config.KnowledgeBases
		}
	}
	// Match the chat retrieval target semantics: a tag scope narrows its parent
	// KB even when that KB is present in the agent's preselected KB list. Other
	// explicitly selected KBs remain additive.
	effectiveKBIDs = excludeSuggestionStrings(effectiveKBIDs, resolvedTags.KnowledgeBaseIDs)

	filteredKBIDs, err := types.FilterKnowledgeBasesForTenantAPIKeyScope(ctx, kbIDs, effectiveKBIDs)
	if err != nil {
		return nil, err
	}
	effectiveKBIDs = filteredKBIDs

	if len(effectiveKBIDs) == 0 && len(knowledgeIDs) == 0 && len(resolvedTags.TagIDsByTenant) == 0 {
		return finalizeStarterSuggestions(curated, nil, starterMode, limit), nil
	}

	// Deduplicate questions we've already collected
	seen := make(map[string]bool)
	for _, q := range curated {
		seen[q.Question] = true
	}

	remaining := limit

	// 3. Collect candidate chunks from both FAQ and Document KBs,
	//    grouped by knowledge_id for diversity.
	//    knowledgeID -> list of questions
	buckets := make(map[string][]types.SuggestedQuestion)

	// Determine query scope
	queryKBIDs := effectiveKBIDs
	queryKnowledgeIDs := knowledgeIDs

	// Fetch a large pool so DB-level random sampling covers multiple documents.
	fetchLimit := remaining * 5
	if fetchLimit < 20 {
		fetchLimit = 20
	}

	// Resolve each KB to the tenant whose chunks should be queried. Cross-tenant
	// KBs reached via an organization share map to the source tenant ID; the chunk
	// rows live under that tenant. Without this grouping a caller in tenant A
	// querying a KB shared from tenant B would hit `tenant_id = A` and get zero
	// rows back — the symptom is "suggested questions never appear for shared KBs".
	scopeKBIDs := mergeUniqueStrings(queryKBIDs, resolvedTags.KnowledgeBaseIDs)
	kbGroups := s.groupKBIDsByEffectiveTenant(ctx, tenantID, scopeKBIDs)
	// Always keep the caller's tenant in the iteration so knowledge_ids-only
	// requests (no kbIDs) still execute one query under the caller's tenant.
	if len(scopeKBIDs) == 0 {
		kbGroups[tenantID] = nil
	}

	// Collect FAQ recommended chunks
	for groupTenantID, groupKBIDs := range kbGroups {
		explicitGroupKBIDs := intersectSuggestionStrings(groupKBIDs, queryKBIDs)
		groupTagIDs := resolvedTags.TagIDsByTenant[groupTenantID]
		faqChunks, err := s.chunkRepo.ListRecommendedFAQChunks(
			ctx, groupTenantID, explicitGroupKBIDs, queryKnowledgeIDs, groupTagIDs, fetchLimit,
		)
		if err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"agent_id":  agentID,
				"tenant_id": groupTenantID,
			})
			continue
		}
		for _, chunk := range faqChunks {
			meta, err := chunk.FAQMetadata()
			if err != nil || meta == nil || meta.StandardQuestion == "" {
				continue
			}
			if seen[meta.StandardQuestion] {
				continue
			}
			seen[meta.StandardQuestion] = true
			buckets[chunk.KnowledgeID] = append(buckets[chunk.KnowledgeID], types.SuggestedQuestion{
				Question:        meta.StandardQuestion,
				Source:          "faq",
				KnowledgeBaseID: chunk.KnowledgeBaseID,
			})
		}
	}

	// Collect Document chunks with generated questions
	for groupTenantID, groupKBIDs := range kbGroups {
		explicitGroupKBIDs := intersectSuggestionStrings(groupKBIDs, queryKBIDs)
		docChunks, err := s.chunkRepo.ListRecentDocumentChunksWithQuestions(ctx, groupTenantID, explicitGroupKBIDs, queryKnowledgeIDs, fetchLimit)
		if err != nil {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"agent_id":  agentID,
				"tenant_id": groupTenantID,
			})
			continue
		}
		for _, chunk := range docChunks {
			meta, err := chunk.DocumentMetadata()
			if err != nil || meta == nil || len(meta.GeneratedQuestions) == 0 {
				continue
			}
			questions := meta.GetQuestionStrings()
			if len(questions) == 0 {
				continue
			}
			q := questions[0]
			if q == "" || seen[q] {
				continue
			}
			seen[q] = true
			buckets[chunk.KnowledgeID] = append(buckets[chunk.KnowledgeID], types.SuggestedQuestion{
				Question:        q,
				Source:          "document",
				KnowledgeBaseID: chunk.KnowledgeBaseID,
			})
		}
	}

	// Collect Wiki pages as a fallback source, but only for KBs the caller selected
	// explicitly. A tag's parent KB is merely an ownership boundary; widening a
	// tag-only scope to arbitrary Wiki pages would make the suggestions unanswerable
	// inside the user's selected range.
	//
	// Skip entirely for quick-answer (RAG-only) agents: those can't ever
	// retrieve a wiki page, so surfacing wiki-derived suggestions would lure
	// the user into asking questions the agent will then answer with empty
	// context. Smart-reasoning agents that opt in to wiki tools keep this.
	if agent.Config.AgentMode != types.AgentModeQuickAnswer && s.wikiPageRepo != nil {
		for groupTenantID, groupKBIDs := range kbGroups {
			explicitGroupKBIDs := intersectSuggestionStrings(groupKBIDs, queryKBIDs)
			if len(explicitGroupKBIDs) == 0 {
				continue
			}
			wikiPages, err := s.wikiPageRepo.ListRecentForSuggestions(ctx, groupTenantID, explicitGroupKBIDs, fetchLimit)
			if err != nil {
				logger.ErrorWithFields(ctx, err, map[string]interface{}{
					"agent_id":  agentID,
					"tenant_id": groupTenantID,
				})
				continue
			}
			locale := types.LanguageFromContextOrDefault(ctx)
			for _, page := range wikiPages {
				q := wikiSuggestionFromPage(page, locale)
				if q == "" || seen[q] {
					continue
				}
				seen[q] = true
				// Use page.ID as the bucket key so round-robin mixes pages from
				// different wiki entries rather than clumping them.
				buckets[page.ID] = append(buckets[page.ID], types.SuggestedQuestion{
					Question:        q,
					Source:          "wiki",
					KnowledgeBaseID: page.KnowledgeBaseID,
				})
			}
		}
	}

	// 4. Shuffle within each bucket, then round-robin across buckets
	//    to ensure diversity across different documents.
	bucketKeys := make([]string, 0, len(buckets))
	for k, qs := range buckets {
		bucketKeys = append(bucketKeys, k)
		rand.Shuffle(len(qs), func(i, j int) { qs[i], qs[j] = qs[j], qs[i] })
		buckets[k] = qs
	}
	rand.Shuffle(len(bucketKeys), func(i, j int) {
		bucketKeys[i], bucketKeys[j] = bucketKeys[j], bucketKeys[i]
	})

	// Round-robin pick one question from each document in turn.
	knowledgeResult := make([]types.SuggestedQuestion, 0, limit)
	offsets := make(map[string]int, len(bucketKeys))
	for len(knowledgeResult) < limit {
		picked := false
		for _, key := range bucketKeys {
			if len(knowledgeResult) >= limit {
				break
			}
			qs := buckets[key]
			idx := offsets[key]
			if idx < len(qs) {
				knowledgeResult = append(knowledgeResult, qs[idx])
				offsets[key] = idx + 1
				picked = true
			}
		}
		if !picked {
			break
		}
	}

	return finalizeStarterSuggestions(curated, knowledgeResult, starterMode, limit), nil
}

type resolvedSuggestionTagScopes struct {
	KnowledgeBaseIDs []string
	KnowledgeIDs     []string
	TagIDsByTenant   map[uint64][]string
}

// resolveSuggestionTagScopes keeps tag ownership separate from whole-KB
// selection. Document tags become concrete knowledge IDs; FAQ tags remain
// chunk tag filters. Scoped inputs also let shared-KB tags resolve against the
// source tenant that owns the tag and chunk rows.
func (s *customAgentService) resolveSuggestionTagScopes(
	ctx context.Context,
	callerTenantID uint64,
	tagScopes []types.TagScope,
) (resolvedSuggestionTagScopes, error) {
	result := resolvedSuggestionTagScopes{TagIDsByTenant: make(map[uint64][]string)}
	if len(tagScopes) == 0 || s.tagRepo == nil || s.knowledgeRepo == nil || s.kbService == nil {
		return result, nil
	}

	byKB := make(map[string][]string)
	for _, scope := range tagScopes {
		if scope.KnowledgeBaseID == "" {
			continue
		}
		for _, tagID := range scope.TagIDs {
			if tagID == "" {
				continue
			}
			byKB[scope.KnowledgeBaseID] = append(byKB[scope.KnowledgeBaseID], tagID)
		}
	}
	if len(byKB) == 0 {
		return result, nil
	}

	kbIDs := make([]string, 0, len(byKB))
	for kbID := range byKB {
		kbIDs = append(kbIDs, kbID)
	}
	kbGroups := s.groupKBIDsByEffectiveTenant(ctx, callerTenantID, kbIDs)
	for tenantID, groupKBIDs := range kbGroups {
		for _, kbID := range groupKBIDs {
			requested := mergeUniqueStrings(nil, byKB[kbID])
			tags, err := s.tagRepo.GetByIDs(ctx, tenantID, requested)
			if err != nil {
				return result, err
			}
			requestedSet := make(map[string]bool, len(requested))
			for _, id := range requested {
				requestedSet[id] = true
			}
			validTagIDs := make([]string, 0, len(tags))
			for _, tag := range tags {
				if tag != nil && tag.KnowledgeBaseID == kbID && requestedSet[tag.ID] {
					validTagIDs = append(validTagIDs, tag.ID)
				}
			}
			if len(validTagIDs) == 0 {
				continue
			}

			result.KnowledgeBaseIDs = mergeUniqueStrings(result.KnowledgeBaseIDs, []string{kbID})
			result.TagIDsByTenant[tenantID] = mergeUniqueStrings(result.TagIDsByTenant[tenantID], validTagIDs)
			knowledgeIDs, err := s.knowledgeRepo.ListIDsByTagIDs(ctx, tenantID, kbID, validTagIDs)
			if err != nil {
				return result, err
			}
			result.KnowledgeIDs = mergeUniqueStrings(result.KnowledgeIDs, knowledgeIDs)
		}
	}
	return result, nil
}

func flattenTagScopeIDs(scopes []types.TagScope) []string {
	var ids []string
	for _, scope := range scopes {
		ids = mergeUniqueStrings(ids, scope.TagIDs)
	}
	return ids
}

func intersectSuggestionStrings(values, allowed []string) []string {
	if len(values) == 0 || len(allowed) == 0 {
		return nil
	}
	allowedSet := make(map[string]bool, len(allowed))
	for _, value := range allowed {
		allowedSet[value] = true
	}
	var result []string
	for _, value := range values {
		if value != "" && allowedSet[value] {
			result = append(result, value)
		}
	}
	return result
}

func excludeSuggestionStrings(values, excluded []string) []string {
	if len(values) == 0 || len(excluded) == 0 {
		return values
	}
	excludedSet := make(map[string]bool, len(excluded))
	for _, value := range excluded {
		excludedSet[value] = true
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !excludedSet[value] {
			result = append(result, value)
		}
	}
	return result
}

func finalizeStarterSuggestions(
	curated []types.SuggestedQuestion,
	knowledge []types.SuggestedQuestion,
	mode string,
	limit int,
) []types.SuggestedQuestion {
	if limit <= 0 {
		return []types.SuggestedQuestion{}
	}
	switch mode {
	case types.SuggestionModeCurated:
		return truncateSuggestedQuestions(curated, limit)
	case types.SuggestionModeHybrid:
		return mergeHybridStarterSuggestions(curated, knowledge, limit)
	default:
		return truncateSuggestedQuestions(knowledge, limit)
	}
}

// mergeHybridStarterSuggestions prioritizes curated starters while reserving
// about one third of visible slots for scope-aware knowledge questions.
func mergeHybridStarterSuggestions(
	curated []types.SuggestedQuestion,
	knowledge []types.SuggestedQuestion,
	limit int,
) []types.SuggestedQuestion {
	if limit <= 0 {
		return []types.SuggestedQuestion{}
	}
	knowledgeSlots := 0
	if limit > 1 {
		knowledgeSlots = (limit + 1) / 3
	}
	curatedSlots := limit - knowledgeSlots
	result := make([]types.SuggestedQuestion, 0, limit)
	seen := make(map[string]bool, limit)
	appendFrom := func(items []types.SuggestedQuestion, max int) {
		added := 0
		for _, item := range items {
			if len(result) == limit || (max >= 0 && added == max) {
				return
			}
			key := strings.ToLower(strings.TrimSpace(item.Question))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, item)
			added++
		}
	}
	appendFrom(curated, curatedSlots)
	appendFrom(knowledge, knowledgeSlots)
	appendFrom(curated, -1)
	appendFrom(knowledge, -1)
	return result
}

func truncateSuggestedQuestions(questions []types.SuggestedQuestion, limit int) []types.SuggestedQuestion {
	if len(questions) > limit {
		return questions[:limit]
	}
	return questions
}

func mergeUniqueStrings(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, s := range base {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, s := range extra {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// truncateQuestions truncates the question list to the specified limit
func (s *customAgentService) truncateQuestions(questions []types.SuggestedQuestion, limit int) []types.SuggestedQuestion {
	if len(questions) > limit {
		return questions[:limit]
	}
	return questions
}

// wikiSuggestionFromPage converts a wiki page into a human-readable suggested
// question string. The template is chosen per page type so the chip reads
// naturally for that kind of content:
//   - concept: "What is <title>?" works for abstract terms (RAG, embedding,
//     idempotency…).
//   - entity / summary: "Tell me about <title>" is neutral and works for
//     people, places, organizations, products and document summaries where
//     "what is <name>?" would read awkwardly ("什么是张三？").
//   - everything else (synthesis, comparison, …): the raw title is already a
//     good topical query on its own.
func wikiSuggestionFromPage(page *types.WikiPage, locale string) string {
	if page == nil {
		return ""
	}
	title := strings.TrimSpace(page.Title)
	if title == "" {
		return ""
	}
	switch page.PageType {
	case types.WikiPageTypeConcept:
		if isEnglishLocale(locale) {
			return "What is " + title + "?"
		}
		return "什么是" + title + "？"
	case types.WikiPageTypeEntity, types.WikiPageTypeSummary:
		if isEnglishLocale(locale) {
			return "Tell me about " + title
		}
		return "介绍一下" + title
	default:
		return title
	}
}

// groupKBIDsByEffectiveTenant resolves each kbID to the tenant whose chunk
// rows back that KB, so cross-tenant shares can be queried correctly:
//   - In-tenant KBs map to the caller's tenant id.
//   - KBs owned by another tenant are included only if the caller's tenant
//     has at least Viewer access via an organization share, in which case
//     the KB maps to its source tenant id (where the chunks actually live).
//   - KBs the caller cannot reach (no membership, no share) are silently
//     dropped — the suggestion endpoint never returns 403, it just shows
//     nothing for that KB, mirroring how search results are scoped.
//
// The result is keyed by effective tenant id so the caller can issue one
// chunk / wiki query per tenant group. Returns an empty (non-nil) map when
// kbIDs is empty.
func (s *customAgentService) groupKBIDsByEffectiveTenant(
	ctx context.Context,
	callerTenantID uint64,
	kbIDs []string,
) map[uint64][]string {
	out := make(map[uint64][]string)
	if len(kbIDs) == 0 {
		return out
	}
	kbs, err := s.kbService.GetKnowledgeBasesByIDsOnly(ctx, kbIDs)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"kb_ids": kbIDs,
		})
		return out
	}
	kbByID := make(map[string]*types.KnowledgeBase, len(kbs))
	for _, kb := range kbs {
		if kb != nil {
			kbByID[kb.ID] = kb
		}
	}
	callerRole := types.TenantRoleFromContext(ctx)
	for _, kbID := range kbIDs {
		kb := kbByID[kbID]
		if kb == nil {
			continue
		}
		if kb.TenantID == callerTenantID {
			governed, err := s.kbService.GetKnowledgeBaseByID(ctx, kbID)
			if err == nil && governed != nil && governed.TenantID == callerTenantID {
				out[callerTenantID] = append(out[callerTenantID], kbID)
			}
			continue
		}
		if s.kbShareService == nil {
			continue
		}
		ok, err := s.kbShareService.HasTenantKBPermission(ctx, kbID, callerTenantID, callerRole, types.OrgRoleViewer)
		if err != nil || !ok {
			continue
		}
		out[kb.TenantID] = append(out[kb.TenantID], kbID)
	}
	return out
}

// isEnglishLocale reports whether the locale string is an English variant.
// Unknown / empty locales fall back to Chinese, matching the product default.
func isEnglishLocale(locale string) bool {
	switch locale {
	case "en-US", "en", "en-GB":
		return true
	}
	return false
}
