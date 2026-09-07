import assert from 'node:assert/strict'
import test from 'node:test'
import type { OperatingMessage, OperatingProcess } from './operatingClient.ts'
import { operatingProcessSession, projectOperatingProcess } from './operatingPresentation.ts'

test('maps snapshot process state into the existing agent event stream shape', () => {
  const process: OperatingProcess = {
    planItems: [
      { id: 'private-plan-id', text: '确认门店范围', status: 'completed' },
      { id: 'private-plan-id-2', text: '比较品类毛利', status: 'in_progress' },
    ],
    queries: [{
      id: 'public-query-call-id',
      intent: '核对门店毛利',
      status: 'ok',
      rowCount: 12,
      queryExecutionId: 'public-query-execution-id',
      detailAvailable: true,
    }],
    calculations: [{ id: 'private-cell-id', status: 'running' }],
  }

  assert.deepEqual(projectOperatingProcess(process), [
    {
      type: 'tool_call',
      tool_call_id: 'operating-plan',
      tool_name: 'todo_write',
      pending: true,
      display_type: 'plan',
      tool_data: {
        display_type: 'plan',
        task: '比较品类毛利',
        steps: [
          { id: 'operating-plan-step-0', description: '确认门店范围', status: 'completed' },
          { id: 'operating-plan-step-1', description: '比较品类毛利', status: 'in_progress' },
        ],
        total_steps: 2,
      },
    },
    {
      type: 'tool_call',
      tool_call_id: 'operating-query-public-query-call-id',
      tool_name: 'database_query',
      pending: false,
      success: true,
      tool_data: {
        intent: '核对门店毛利',
        status: 'succeeded',
        query_execution_id: 'public-query-execution-id',
        result_available: true,
        row_count: 12,
      },
    },
    {
      type: 'tool_call',
      tool_call_id: 'operating-calculation-private-cell-id',
      tool_name: 'data_analysis',
      pending: true,
      tool_data: { status: 'running' },
    },
  ])
})

test('projects only public process fields and never copies raw reasoning, SQL or JSON', () => {
  const process = {
    planItems: [{ id: 'plan-secret', text: '核对销售趋势', status: 'done', reasoning: 'hidden-chain' }],
    queries: [{
      id: 'query-secret',
      intent: '查看周销售额',
      status: 'completed',
      rowCount: 3,
      sql: 'select secret from private_table',
      raw: { customer: 'private-json' },
    }],
    calculations: [{ id: 'cell-secret', status: 'succeeded', code: 'print(private_value)' }],
  } as unknown as OperatingProcess
  const projected = JSON.stringify(projectOperatingProcess(process))

  assert.match(projected, /核对销售趋势/)
  assert.match(projected, /查看周销售额/)
  for (const secret of ['plan-secret', 'hidden-chain', 'private_table', 'private-json', 'private_value']) {
    assert.doesNotMatch(projected, new RegExp(secret))
  }
})

test('message terminal status stays outside the projected event content', () => {
  const message: OperatingMessage = {
    id: 'message-1',
    role: 'assistant',
    text: '',
    createdAt: null,
    status: 'cancelled',
    process: { planItems: [], queries: [], calculations: [] },
    report: null,
  }
  assert.deepEqual(operatingProcessSession(message), { agentEventStream: [] })
})
