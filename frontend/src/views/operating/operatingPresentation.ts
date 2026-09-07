import type { OperatingMessage, OperatingProcess } from './operatingClient'

type AgentProcessEvent = {
  type: 'tool_call'
  tool_call_id: string
  tool_name: 'todo_write' | 'database_query' | 'data_analysis'
  pending: boolean
  success?: boolean
  display_type?: 'plan'
  tool_data: Record<string, unknown>
}

const completedStatuses = new Set(['completed', 'done', 'succeeded', 'success', 'ok'])
const runningStatuses = new Set(['running', 'active', 'in_progress', 'started', 'pending'])

function stepStatus(status: string): 'pending' | 'in_progress' | 'completed' | 'skipped' {
  const normalized = status.toLowerCase()
  if (completedStatuses.has(normalized)) return 'completed'
  if (runningStatuses.has(normalized) && normalized !== 'pending') return 'in_progress'
  if (normalized === 'failed' || normalized === 'cancelled' || normalized === 'skipped') return 'skipped'
  return 'pending'
}

function operationState(status?: string): 'running' | 'succeeded' | 'failed' {
  const normalized = status?.toLowerCase() ?? ''
  if (completedStatuses.has(normalized)) return 'succeeded'
  if (normalized === 'failed' || normalized === 'cancelled' || normalized === 'rejected') return 'failed'
  return 'running'
}

export function projectOperatingProcess(process: OperatingProcess): AgentProcessEvent[] {
  const events: AgentProcessEvent[] = []

  if (process.planItems.length > 0) {
    const steps = process.planItems.map((item, index) => ({
      id: `operating-plan-step-${index}`,
      description: item.text,
      status: stepStatus(item.status),
    }))
    const activeStep = steps.find((step) => step.status === 'in_progress')
      ?? [...steps].reverse().find((step) => step.status === 'completed')
      ?? steps[0]
    const pending = steps.some((step) => step.status === 'in_progress' || step.status === 'pending')
    events.push({
      type: 'tool_call',
      tool_call_id: 'operating-plan',
      tool_name: 'todo_write',
      pending,
      ...(!pending ? { success: !steps.some((step) => step.status === 'skipped') } : {}),
      display_type: 'plan',
      tool_data: {
        display_type: 'plan',
        task: activeStep.description,
        steps,
        total_steps: steps.length,
      },
    })
  }

  process.queries.forEach((query) => {
    const status = operationState(query.status)
    events.push({
      type: 'tool_call',
      tool_call_id: `operating-query-${query.id}`,
      tool_name: 'database_query',
      pending: status === 'running',
      ...(status !== 'running' ? { success: status === 'succeeded' } : {}),
      tool_data: {
        intent: query.intent,
        status,
        ...(query.queryExecutionId ? { query_execution_id: query.queryExecutionId } : {}),
        ...(query.detailAvailable ? { result_available: true } : {}),
        ...(typeof query.rowCount === 'number' && Number.isFinite(query.rowCount)
          ? { row_count: Math.max(0, Math.trunc(query.rowCount)) }
          : {}),
      },
    })
  })

  process.calculations.forEach((calculation) => {
    const status = operationState(calculation.status)
    events.push({
      type: 'tool_call',
      tool_call_id: `operating-calculation-${calculation.id}`,
      tool_name: 'data_analysis',
      pending: status === 'running',
      ...(status !== 'running' ? { success: status === 'succeeded' } : {}),
      tool_data: { status },
    })
  })

  return events
}

export function operatingProcessSession(message: OperatingMessage) {
  return { agentEventStream: projectOperatingProcess(message.process) }
}
