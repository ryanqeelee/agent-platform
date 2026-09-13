package handler

import (
	"context"
	stderrors "errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/config"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
)

type platformAgentManager interface {
	List(context.Context) ([]*types.CustomAgent, error)
	Get(context.Context, string) (*types.CustomAgent, error)
	Update(context.Context, string, types.CustomAgentConfig) (*types.CustomAgent, error)
}

// PlatformAgentHandler exposes tenantless management of global built-in agent
// definitions. Enterprise resource binding remains on the runtime read path.
type PlatformAgentHandler struct {
	service platformAgentManager
	config  *config.Config
}

func NewPlatformAgentHandler(service *service.PlatformAgentService, cfg *config.Config) *PlatformAgentHandler {
	return &PlatformAgentHandler{service: service, config: cfg}
}

func (h *PlatformAgentHandler) List(c *gin.Context) {
	agents, err := h.service.List(c.Request.Context())
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": agents})
}

func (h *PlatformAgentHandler) Get(c *gin.Context) {
	agent, err := h.service.Get(c.Request.Context(), strings.TrimSpace(c.Param("id")))
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": agent})
}

type UpdatePlatformAgentRequest struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Avatar      string                  `json:"avatar"`
	Config      types.CustomAgentConfig `json:"config"`
}

func (h *PlatformAgentHandler) Update(c *gin.Context) {
	var req UpdatePlatformAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.NewBadRequestError("Invalid request parameters").WithDetails(err.Error()))
		return
	}
	agent, err := h.service.Update(c.Request.Context(), strings.TrimSpace(c.Param("id")), req.Config)
	if err != nil {
		h.handleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": agent})
}

func (h *PlatformAgentHandler) GetPlaceholders(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"all":                   types.AllPlaceholders(),
			"system_prompt":         types.PlaceholdersByField(types.PromptFieldSystemPrompt),
			"agent_system_prompt":   types.PlaceholdersByField(types.PromptFieldAgentSystemPrompt),
			"context_template":      types.PlaceholdersByField(types.PromptFieldContextTemplate),
			"rewrite_system_prompt": types.PlaceholdersByField(types.PromptFieldRewriteSystemPrompt),
			"rewrite_prompt":        types.PlaceholdersByField(types.PromptFieldRewritePrompt),
			"fallback_prompt":       types.PlaceholdersByField(types.PromptFieldFallbackPrompt),
		},
	})
}

func (h *PlatformAgentHandler) GetTypePresets(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    types.ListAgentTypePresetsWithContext(c.Request.Context()),
	})
}

func (h *PlatformAgentHandler) GetPromptTemplates(c *gin.Context) {
	templates := h.config.PromptTemplates
	if templates == nil {
		templates = &config.PromptTemplatesConfig{}
	}
	lang := types.LanguageFromContextOrDefault(c.Request.Context())
	localized := &config.PromptTemplatesConfig{
		SystemPrompt:         config.LocalizeTemplates(templates.SystemPrompt, lang),
		ContextTemplate:      config.LocalizeTemplates(templates.ContextTemplate, lang),
		Rewrite:              config.LocalizeTemplates(templates.Rewrite, lang),
		Fallback:             config.LocalizeTemplates(templates.Fallback, lang),
		GenerateSessionTitle: templates.GenerateSessionTitle,
		GenerateSummary:      templates.GenerateSummary,
		KeywordsExtraction:   templates.KeywordsExtraction,
		AgentSystemPrompt:    config.LocalizeTemplates(templates.AgentSystemPrompt, lang),
		IntentPrompts:        config.LocalizeTemplates(templates.IntentPrompts, lang),
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": localized})
}

func (h *PlatformAgentHandler) handleError(c *gin.Context, err error) {
	switch {
	case stderrors.Is(err, service.ErrPlatformAgentForbidden):
		c.Error(apperrors.NewForbiddenError(err.Error()))
	case stderrors.Is(err, service.ErrPlatformAgentNotFound):
		c.Error(apperrors.NewNotFoundError(err.Error()))
	case stderrors.Is(err, service.ErrPlatformAgentInvalidConfig):
		c.Error(apperrors.NewBadRequestError(err.Error()))
	default:
		c.Error(apperrors.NewInternalServerError(err.Error()))
	}
}
