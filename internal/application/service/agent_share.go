package service

import (
	"errors"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

var (
	ErrAgentShareNotFound      = errors.New("agent share not found")
	ErrAgentSharePermission    = errors.New("permission denied for this share operation")
	ErrAgentNotFoundForShare   = errors.New("agent not found")
	ErrNotAgentOwner           = errors.New("only agent owner can share")
	ErrOrgRoleCannotShareAgent = errors.New("only editors and admins can share agents to this organization")
	ErrAgentNotConfigured      = errors.New("agent is not fully configured (missing required chat model, or rerank model when the knowledge_search tool is enabled)")
)

// agentRequiresRerankModel mirrors the active Agent runtime: only a usable
// knowledge_search path needs a reranker.
func agentRequiresRerankModel(agent *types.CustomAgent) bool {
	if agent == nil || agent.Config.KBSelectionMode == "none" {
		return false
	}
	allowed := agent.Config.AllowedTools
	if len(allowed) == 0 {
		allowed = tools.DefaultAllowedTools()
	}
	for _, tool := range allowed {
		if tool == tools.ToolKnowledgeSearch {
			return true
		}
	}
	return false
}

// NewAgentShareService keeps the existing interface wiring while the platform,
// rather than enterprise users, owns cross-enterprise sharing.
func NewAgentShareService() interfaces.AgentShareService {
	return enterpriseManagedAgentShareService{}
}
