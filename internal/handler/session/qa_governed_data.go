package session

import (
	"context"
	"slices"

	"github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/application/service"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

// Only the active interactive QA turn carries its user's external-data
// credential. Stop watchers, knowledge search and detached workers do not.
func cloneInteractiveQATurn(ctx context.Context) context.Context {
	cloned := logger.CloneContext(ctx)
	if !types.GovernedDataObservability(ctx) {
		return cloned
	}
	return types.CopyGovernedDataTurnCredential(cloned, ctx)
}

func agentRequiresGovernedAdmission(agent *types.CustomAgent) bool {
	if agent == nil {
		return false
	}
	if agent.ID == types.BuiltinOperatingAnalystID {
		return true
	}
	for _, name := range agent.Config.AllowedTools {
		if name == tools.ToolGovernedDataSchema || name == tools.ToolGovernedDataQuery {
			return true
		}
	}
	return false
}

// Registration requires both tools. A partial allowlist still requires admission,
// but cannot justify propagating the credential into QA execution.
func agentCanConsumeGovernedData(agent *types.CustomAgent) bool {
	return agent != nil && slices.Contains(agent.Config.AllowedTools, tools.ToolGovernedDataSchema) &&
		slices.Contains(agent.Config.AllowedTools, tools.ToolGovernedDataQuery)
}

func messagesContainGovernedData(messages []*types.Message) bool {
	for _, message := range messages {
		if message == nil || message.Role != "assistant" {
			continue
		}
		if message.AgentID == types.BuiltinOperatingAnalystID {
			return true
		}
		for _, step := range message.AgentSteps {
			for _, call := range step.ToolCalls {
				if call.Name == tools.ToolGovernedDataSchema || call.Name == tools.ToolGovernedDataQuery {
					return true
				}
			}
		}
	}
	return false
}

// sessionHasGovernedHistory checks only the same bounded history window that
// the effective QA configuration can replay into a model call.
func (h *Handler) sessionHasGovernedHistory(
	ctx context.Context, sessionID string, agent *types.CustomAgent, agentMode bool,
) (bool, error) {
	maxRounds := 0
	if agent != nil {
		if !agent.Config.MultiTurnEnabled {
			return false, nil
		}
		maxRounds = agent.Config.HistoryTurns
		if maxRounds <= 0 && agentMode {
			// AgentQA's history loader applies this same default.
			maxRounds = 5
		}
		if maxRounds <= 0 && h.config != nil {
			maxRounds = h.config.Conversation.MaxRounds
		}
	} else if h.config != nil {
		maxRounds = h.config.Conversation.MaxRounds
	}
	if maxRounds <= 0 || h.messageService == nil {
		return false, nil
	}

	// Match the largest history fetch used by the selected execution path.
	// The agent loader has a floor for incomplete pairs; the normal pipeline
	// also has a fixed 20-row query-understanding fetch.
	limit := maxRounds*2 + 10
	if agentMode {
		limit = maxRounds * 4
		if limit < 50 {
			limit = 50
		}
	} else if limit < 20 {
		limit = 20
	}
	messages, err := h.messageService.GetRecentMessagesBySession(ctx, sessionID, limit)
	if err != nil {
		return false, err
	}
	return messagesContainGovernedData(messages), nil
}

// Recheck current authorization before a turn can consume staged business files.
func (h *Handler) authorizeGovernedAgent(ctx context.Context, agent *types.CustomAgent) error {
	if !agentRequiresGovernedAdmission(agent) {
		return nil
	}
	_, err := service.AuthorizeGovernedData(ctx, h.userService, h.tenantMemberService, h.tenantService, h.governedEdgeResolver)
	return err
}
