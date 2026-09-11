package agent

import (
	"context"
	"fmt"
	"time"

	agenttools "github.com/Tencent/WeKnora/internal/agent/tools"
	"github.com/Tencent/WeKnora/internal/common"
	"github.com/Tencent/WeKnora/internal/event"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/searchutil"
	"github.com/Tencent/WeKnora/internal/types"
)

func finalAnswerImageRequirement(hasRetrievedImage bool) string {
	if !hasRetrievedImage {
		return ""
	}
	return agentRetrievedImageSystemRequirement
}

// streamFinalAnswerToEventBus streams the final answer generation through EventBus
func (e *AgentEngine) streamFinalAnswerToEventBus(
	ctx context.Context,
	query string,
	messages []chat.Message,
	state *types.AgentState,
	sessionID string,
) error {
	totalToolCalls := countTotalToolCalls(state.RoundSteps)
	logger.Infof(ctx, "[Agent][FinalAnswer] Synthesizing from %d steps, %d tool calls",
		len(state.RoundSteps), totalToolCalls)
	finalFields := map[string]interface{}{
		"session_id":   sessionID,
		"steps":        len(state.RoundSteps),
		"tool_results": totalToolCalls,
	}
	if types.GovernedDataObservability(ctx) {
		finalFields["query_len"] = len(query)
	} else {
		finalFields["query"] = query
	}
	common.PipelineInfo(ctx, "Agent", "final_answer_start", finalFields)

	// Finalization is another model turn, not a new conversation. Reuse the
	// current (possibly compacted) transcript so user constraints, attachments
	// and tool provenance survive the iteration/error boundary. In particular,
	// do not resurrect old RoundSteps as user-authored evidence.
	messages = append([]chat.Message(nil), messages...)
	hasRetrievedImage := false
	for _, message := range messages {
		if message.Role == "tool" && searchutil.MarkdownImageRegex.MatchString(message.Content) {
			hasRetrievedImage = true
			break
		}
	}

	imageRequirement := finalAnswerImageRequirement(hasRetrievedImage)

	// Add final answer prompt
	finalPrompt := fmt.Sprintf(`Complete the user's task using the conversation and available evidence above. Preserve the user's confirmed constraints and distinguish tool evidence from user instructions.

User question: %s

Requirements:
1. Use only evidence that applies to the requested entity, conditions and scope. Related examples do not establish facts about this case.
2. Organize the answer in a structured format
3. If evidence is insufficient to choose a correct action, state the gap and ask the essential clarification. Do not fill it with an unrelated example or an unsupported specific claim.
4. IMPORTANT: Respond in the same language as the user's question
%s

Now generate the final answer:`, query, imageRequirement)

	messages = append(messages, chat.Message{
		Role:    "user",
		Content: finalPrompt,
	})

	// The last tool result may have filled the window after the final loop
	// iteration. Use the same compaction policy as an ordinary model turn.
	messages, _ = e.manageContextWindow(ctx, messages, state.CurrentRound+1, e.tokenEstimator.EstimateMessages(messages))
	messages = agenttools.SanitizeMessages(messages)

	// Generate a single ID for this entire final answer stream
	answerID := generateEventID("answer")
	logger.Debugf(ctx, "[Agent][FinalAnswer] AnswerID: %s", answerID)
	answerDoneEmitted := false

	budget := e.clampCompletionBudgetToContext(e.tokenEstimator.EstimateMessages(messages))
	finalOptions := &chat.ChatOptions{
		Temperature:         e.config.Temperature,
		MaxTokens:           budget,
		MaxCompletionTokens: budget,
		PromptCacheKey:      sessionID,
	}
	if e.completionOptions != nil {
		prepared := *e.completionOptions
		prepared.Tools = nil
		prepared.ToolChoice = ""
		prepared.ParallelToolCalls = nil
		prepared.Thinking = nil
		prepared.PromptCacheKey = sessionID
		if prepared.MaxCompletionTokens <= 0 || prepared.MaxCompletionTokens > budget {
			prepared.MaxCompletionTokens = budget
		}
		if prepared.MaxTokens <= 0 || prepared.MaxTokens > budget {
			prepared.MaxTokens = prepared.MaxCompletionTokens
		}
		finalOptions = &prepared
	}
	llmResult, err := e.streamLLMToEventBus(
		ctx,
		messages,
		finalOptions, // Thinking disabled for final answer synthesis
		func(chunk *types.StreamResponse, fullContent string) {
			// Defensive filter: only emit answer content, skip thinking chunks
			if chunk.ResponseType == types.ResponseTypeThinking {
				return
			}
			if chunk.Content != "" {
				logger.Debugf(ctx, "[Agent][FinalAnswer] Emitting answer chunk: %d chars", len(chunk.Content))
				e.eventBus.Emit(ctx, event.Event{
					ID:        answerID,
					Type:      event.EventAgentFinalAnswer,
					SessionID: sessionID,
					Data: event.AgentFinalAnswerData{
						Content: chunk.Content,
						Done:    chunk.Done,
					},
				})
				if chunk.Done {
					answerDoneEmitted = true
				}
			}
		},
	)
	if err != nil {
		failedFields := map[string]interface{}{"session_id": sessionID}
		if types.GovernedDataObservability(ctx) {
			logger.Errorf(ctx, "[Agent][FinalAnswer] Final answer generation failed: error_payload_omitted=true")
			failedFields["has_error"] = true
		} else {
			logger.Errorf(ctx, "[Agent][FinalAnswer] Final answer generation failed: %v", err)
			failedFields["error"] = err.Error()
		}
		common.PipelineError(ctx, "Agent", "final_answer_stream_failed", failedFields)
		return err
	}

	if !answerDoneEmitted {
		e.eventBus.Emit(ctx, event.Event{
			ID:        answerID,
			Type:      event.EventAgentFinalAnswer,
			SessionID: sessionID,
			Data: event.AgentFinalAnswerData{
				Content: "",
				Done:    true,
			},
		})
	}

	// The synthesis call is often the largest of the turn — fold its usage
	// into the turn aggregate like every ReAct round.
	if llmResult.Usage != nil {
		state.TurnUsage.Accumulate(*llmResult.Usage)
	}

	// Safety net: strip any residual <think> blocks that may have leaked through
	fullAnswer := agenttools.StripThinkBlocks(llmResult.Content)
	logger.Infof(ctx, "[Agent][FinalAnswer] Final answer generated: %d characters", len(fullAnswer))
	common.PipelineInfo(ctx, "Agent", "final_answer_done", map[string]interface{}{
		"session_id": sessionID,
		"answer_len": len(fullAnswer),
	})
	state.FinalAnswer = fullAnswer
	return nil
}

// handleMaxIterations generates a final answer when the agent loop exhausted all iterations
// without the LLM producing a natural stop. It marks state.IsComplete = true.
func (e *AgentEngine) handleMaxIterations(
	ctx context.Context, query string, messages []chat.Message, state *types.AgentState, sessionID string,
) error {
	logger.Info(ctx, "Reached max iterations, generating final answer")
	common.PipelineWarn(ctx, "Agent", "max_iterations_reached", map[string]interface{}{
		"iterations": state.CurrentRound,
		"max":        e.config.MaxIterations,
	})

	// Stream final answer generation through EventBus
	if err := e.streamFinalAnswerToEventBus(ctx, query, messages, state, sessionID); err != nil {
		failedFields := map[string]interface{}{}
		if types.GovernedDataObservability(ctx) {
			logger.Errorf(ctx, "Failed to synthesize final answer: error_payload_omitted=true")
			failedFields["has_error"] = true
		} else {
			logger.Errorf(ctx, "Failed to synthesize final answer: %v", err)
			failedFields["error"] = err.Error()
		}
		common.PipelineError(ctx, "Agent", "final_answer_failed", failedFields)
		return err
	}
	state.IsComplete = true
	return nil
}

// emitCompletionEvent emits the EventAgentComplete event with execution summary.
func (e *AgentEngine) emitCompletionEvent(
	ctx context.Context, state *types.AgentState, sessionID, messageID string, startTime time.Time, outcome string,
) {
	// Convert knowledge refs to interface{} slice for event data
	knowledgeRefsInterface := make([]interface{}, 0, len(state.KnowledgeRefs))
	for _, ref := range state.KnowledgeRefs {
		knowledgeRefsInterface = append(knowledgeRefsInterface, ref)
	}

	e.eventBus.Emit(ctx, event.Event{
		ID:        generateEventID("complete"),
		Type:      event.EventAgentComplete,
		SessionID: sessionID,
		Data: event.AgentCompleteData{
			Outcome:         outcome,
			FinalAnswer:     state.FinalAnswer,
			KnowledgeRefs:   knowledgeRefsInterface,
			AgentSteps:      state.RoundSteps, // Include detailed execution steps for message storage
			Usage:           turnUsage(state),
			TotalSteps:      len(state.RoundSteps),
			TotalDurationMs: time.Since(startTime).Milliseconds(),
			MessageID:       messageID, // Include message ID for proper message update
		},
	})

	logger.Infof(ctx, "Agent execution completed in %d rounds", state.CurrentRound)
}

// turnUsage returns the turn's aggregated LLM usage, or nil when no round
// reported usage so the field stays absent from the completion event and the
// persisted message alike.
func turnUsage(state *types.AgentState) *types.TokenUsage {
	if state == nil || state.TurnUsage.TotalTokens == 0 {
		return nil
	}
	usage := state.TurnUsage
	return &usage
}
