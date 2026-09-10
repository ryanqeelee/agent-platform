package session

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/Tencent/WeKnora/internal/agent/tools"
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
	cloned = types.CopyGovernedDataTurnCredential(cloned, ctx)
	return tools.CopyGovernedAnalysisClient(cloned, ctx)
}

func (h *Handler) startGovernedAnalysisTurn(ctx context.Context, sessionID, turnID, question string) (context.Context, error) {
	bearer, tenantID, ok := types.GovernedDataUserCredential(ctx)
	if !ok || h.config == nil || h.config.Agent == nil || h.config.Agent.GovernedData == nil {
		return ctx, tools.ErrGovernedDataAccessDenied
	}
	client, err := tools.NewGovernedDataClient(h.config.Agent.GovernedData.BaseURL, bearer, strconv.FormatUint(tenantID, 10), "")
	if err != nil {
		return ctx, err
	}
	if err := client.Start(ctx, sessionID, turnID, question); err != nil {
		return ctx, err
	}
	return tools.WithGovernedAnalysisClient(ctx, client), nil
}

func terminateAbandonedGovernedAnalysis(ctx context.Context) {
	client, governed := tools.GovernedAnalysisClientFromContext(ctx)
	if !governed || client.IsTerminal() {
		return
	}
	state := "failed"
	code := "native_turn_abandoned"
	if ctx.Err() != nil {
		state = "cancelled"
		code = "native_turn_cancelled"
	}
	_ = client.Terminate(context.WithoutCancel(ctx), state, code, "native turn exited without Center terminal")
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

// Check the same live Center admission used by the product page before a
// governed turn can consume even previously staged workspace files.
func (h *Handler) authorizeGovernedAgent(ctx context.Context, agent *types.CustomAgent) error {
	if !agentRequiresGovernedAdmission(agent) {
		return nil
	}
	bearer, tenantID, ok := types.GovernedDataUserCredential(ctx)
	if !ok {
		return tools.ErrGovernedDataAccessDenied
	}
	if h.config == nil || h.config.Agent == nil || h.config.Agent.GovernedData == nil {
		return fmt.Errorf("经营分析数据服务未配置")
	}
	client, err := tools.NewGovernedDataClient(h.config.Agent.GovernedData.BaseURL, bearer, strconv.FormatUint(tenantID, 10), "")
	if err != nil {
		return err
	}
	return client.CheckAccess(ctx)
}
