export type AgentHistoryEvent = Record<string, unknown>

export function reconstructEventStreamFromSteps(
  agentSteps: unknown[],
  messageContent: string,
  isCompleted = false,
  isFallback = false,
  agentDurationMs = 0,
  usage?: unknown,
) {
  const events: AgentHistoryEvent[] = []

  if (agentSteps && Array.isArray(agentSteps) && agentSteps.length > 0) {
    agentSteps.forEach((rawStep) => {
      const step = rawStep as AgentHistoryEvent
      const stepTimestamp = step.timestamp ? new Date(String(step.timestamp)).getTime() : 0
      const toolCalls = step.tool_calls
      const hasToolCalls = toolCalls && Array.isArray(toolCalls) && toolCalls.length > 0

      const reasoningText =
        step.reasoning_content && String(step.reasoning_content).trim()
          ? String(step.reasoning_content)
          : ''
      if (reasoningText) {
        events.push({
          type: 'thinking',
          event_id: `step-${step.iteration}-thought`,
          content: reasoningText,
          done: true,
          thinking: false,
          timestamp: stepTimestamp || undefined,
          duration_ms: step.duration || undefined,
        })
      }
      const preambleText = step.thought && String(step.thought).trim() ? String(step.thought) : ''
      if (preambleText && hasToolCalls) {
        events.push({
          type: 'answer',
          event_id: `step-${step.iteration}-preamble`,
          content: preambleText,
          done: true,
          superseded: true,
          timestamp: stepTimestamp || undefined,
        })
      }

      if (toolCalls && Array.isArray(toolCalls)) {
        toolCalls.forEach((toolCall: AgentHistoryEvent) => {
          if (toolCall.name === 'final_answer') return
          const result = toolCall.result as AgentHistoryEvent | undefined
          const resultData = result?.data as AgentHistoryEvent | undefined
          events.push({
            type: 'tool_call',
            tool_call_id: toolCall.id,
            tool_name: toolCall.name,
            arguments: toolCall.args,
            pending: false,
            success: result?.success !== false,
            output: result?.output || '',
            error: result?.error || undefined,
            timestamp: stepTimestamp || undefined,
            duration: toolCall.duration,
            duration_ms: toolCall.duration,
            display_type: resultData?.display_type,
            tool_data: result?.data,
          })
        })
      }
    })
  }

  if (agentDurationMs > 0 || usage) {
    events.push({
      type: 'agent_complete',
      total_duration_ms: agentDurationMs,
      usage,
    })
  }

  if (messageContent && messageContent.trim()) {
    const answerEvent: AgentHistoryEvent = {
      type: 'answer',
      content: messageContent,
      done: true,
    }
    if (isFallback) answerEvent.is_fallback = true
    events.push(answerEvent)
  } else if (isCompleted) {
    events.push({
      type: 'stop',
      timestamp: Date.now(),
      reason: 'user_requested',
    })
  }

  return events
}
