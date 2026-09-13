package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

const platformAgentTenantID uint64 = 0

var (
	ErrPlatformAgentForbidden     = errors.New("platform agent management requires a system administrator")
	ErrPlatformAgentNotFound      = errors.New("platform agent definition not found")
	ErrPlatformAgentInvalidConfig = errors.New("invalid platform agent configuration")
)

// PlatformAgentService owns global overrides for built-in agent definitions.
// Enterprise resources are deliberately absent from this service: runtime
// binding remains in customAgentService under the authenticated tenant.
type PlatformAgentService struct {
	agents interfaces.CustomAgentRepository
	models interfaces.ModelRepository
}

func NewPlatformAgentService(
	agents interfaces.CustomAgentRepository,
	models interfaces.ModelRepository,
) *PlatformAgentService {
	return &PlatformAgentService{agents: agents, models: models}
}

func requirePlatformAgentAdmin(ctx context.Context) error {
	if !types.IsSystemAdminFromContext(ctx) {
		return ErrPlatformAgentForbidden
	}
	return nil
}

func (s *PlatformAgentService) List(ctx context.Context) ([]*types.CustomAgent, error) {
	if err := requirePlatformAgentAdmin(ctx); err != nil {
		return nil, err
	}
	result := make([]*types.CustomAgent, 0, len(types.GetPlatformManagedBuiltinAgentIDs()))
	for _, id := range types.GetPlatformManagedBuiltinAgentIDs() {
		agent, err := s.get(ctx, id)
		if errors.Is(err, ErrPlatformAgentNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, agent)
	}
	return result, nil
}

func (s *PlatformAgentService) Get(ctx context.Context, id string) (*types.CustomAgent, error) {
	if err := requirePlatformAgentAdmin(ctx); err != nil {
		return nil, err
	}
	return s.get(ctx, id)
}

func (s *PlatformAgentService) get(ctx context.Context, id string) (*types.CustomAgent, error) {
	if !types.IsPlatformManagedBuiltinAgentID(id) {
		return nil, ErrPlatformAgentNotFound
	}
	stored, err := s.agents.GetAgentByID(ctx, id, platformAgentTenantID)
	if err == nil {
		stored.TenantID = platformAgentTenantID
		stored.IsBuiltin = true
		stored.EnsureDefaults()
		types.ApplyBuiltinAgentLocalization(ctx, stored)
		return stored, nil
	}
	if !errors.Is(err, repository.ErrCustomAgentNotFound) {
		return nil, err
	}
	fallback := types.GetBuiltinAgentWithContext(ctx, id, platformAgentTenantID)
	if fallback == nil {
		return nil, ErrPlatformAgentNotFound
	}
	return fallback, nil
}

// Update persists a full, tenantless config override. Built-in display identity
// remains YAML-owned and localized; callers edit execution configuration only.
func (s *PlatformAgentService) Update(
	ctx context.Context,
	id string,
	config types.CustomAgentConfig,
) (*types.CustomAgent, error) {
	if err := requirePlatformAgentAdmin(ctx); err != nil {
		return nil, err
	}
	fallback := types.GetBuiltinAgentWithContext(ctx, id, platformAgentTenantID)
	if !types.IsPlatformManagedBuiltinAgentID(id) || fallback == nil {
		return nil, ErrPlatformAgentNotFound
	}
	if err := s.validateConfig(ctx, config); err != nil {
		return nil, err
	}

	now := time.Now()
	stored, err := s.agents.GetAgentByID(ctx, id, platformAgentTenantID)
	switch {
	case err == nil:
		stored.Name = fallback.Name
		stored.Description = fallback.Description
		stored.Avatar = fallback.Avatar
		stored.IsBuiltin = true
		stored.TenantID = platformAgentTenantID
		stored.Config = config
		stored.UpdatedAt = now
		stored.EnsureDefaults()
		if err := stored.Config.QuestionSuggestions.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrPlatformAgentInvalidConfig, err)
		}
		if err := s.agents.UpdateAgent(ctx, stored); err != nil {
			return nil, err
		}
		return stored, nil
	case !errors.Is(err, repository.ErrCustomAgentNotFound):
		return nil, err
	}

	created := &types.CustomAgent{
		ID:          fallback.ID,
		Name:        fallback.Name,
		Description: fallback.Description,
		Avatar:      fallback.Avatar,
		IsBuiltin:   true,
		TenantID:    platformAgentTenantID,
		Config:      config,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	created.EnsureDefaults()
	if err := created.Config.QuestionSuggestions.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPlatformAgentInvalidConfig, err)
	}
	if err := s.agents.CreateAgent(ctx, created); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *PlatformAgentService) validateConfig(ctx context.Context, config types.CustomAgentConfig) error {
	invalidBindings := make([]string, 0, 8)
	if len(config.KnowledgeBases) > 0 {
		invalidBindings = append(invalidBindings, "knowledge_bases")
	}
	if strings.EqualFold(strings.TrimSpace(config.KBSelectionMode), "selected") {
		invalidBindings = append(invalidBindings, "kb_selection_mode")
	}
	if config.SandboxConfigID != "" {
		invalidBindings = append(invalidBindings, "sandbox_config_id")
	}
	if config.WebSearchProviderID != "" {
		invalidBindings = append(invalidBindings, "web_search_provider_id")
	}
	if config.ImageStorageProvider != "" {
		invalidBindings = append(invalidBindings, "image_storage_provider")
	}
	if len(config.MCPServices) > 0 {
		invalidBindings = append(invalidBindings, "mcp_services")
	}
	if len(config.SelectedSkills) > 0 {
		invalidBindings = append(invalidBindings, "selected_skills")
	}
	if len(config.ChatParserEngineRules) > 0 {
		invalidBindings = append(invalidBindings, "chat_parser_engine_rules")
	}
	if strings.EqualFold(strings.TrimSpace(config.MCPSelectionMode), "selected") {
		invalidBindings = append(invalidBindings, "mcp_selection_mode")
	}
	if strings.EqualFold(strings.TrimSpace(config.SkillsSelectionMode), "selected") {
		invalidBindings = append(invalidBindings, "skills_selection_mode")
	}
	if len(invalidBindings) > 0 {
		return fmt.Errorf("%w: tenant resource bindings are not allowed: %s",
			ErrPlatformAgentInvalidConfig, strings.Join(invalidBindings, ", "))
	}

	modelRefs := map[string]struct {
		id    string
		type_ types.ModelType
	}{
		"model_id":                  {config.ModelID, types.ModelTypeKnowledgeQA},
		"rerank_model_id":           {config.RerankModelID, types.ModelTypeRerank},
		"vlm_model_id":              {config.VLMModelID, types.ModelTypeVLLM},
		"asr_model_id":              {config.ASRModelID, types.ModelTypeASR},
		"query_understand_model_id": {config.QueryUnderstandModelID, types.ModelTypeKnowledgeQA},
	}
	if config.QuestionSuggestions != nil {
		modelRefs["question_suggestions.follow_ups.model_id"] = struct {
			id    string
			type_ types.ModelType
		}{config.QuestionSuggestions.FollowUps.ModelID, types.ModelTypeKnowledgeQA}
	}
	for field, ref := range modelRefs {
		if strings.TrimSpace(ref.id) == "" {
			continue
		}
		model, err := s.models.GetByID(ctx, platformAgentTenantID, ref.id)
		if err != nil {
			return err
		}
		if model == nil || (!model.IsBuiltin && model.TenantID != platformAgentTenantID) {
			return fmt.Errorf("%w: %s must reference a global model", ErrPlatformAgentInvalidConfig, field)
		}
		if model.Type != ref.type_ {
			return fmt.Errorf("%w: %s must reference a %s model", ErrPlatformAgentInvalidConfig, field, ref.type_)
		}
		if model.Status != types.ModelStatusActive {
			return fmt.Errorf("%w: %s must reference an active model", ErrPlatformAgentInvalidConfig, field)
		}
	}
	return nil
}
