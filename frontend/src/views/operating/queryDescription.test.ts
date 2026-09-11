import assert from 'node:assert/strict'
import test from 'node:test'
import { queryDescription } from './queryDescription.ts'

test('only successful query results show row counts; rejection and failure stay distinct', () => {
  assert.equal(queryDescription({ pending: true }), '核对经营数据…')
  assert.equal(queryDescription({ success: false, tool_data: { result: { status: 'rejected', row_count: 0 } } }), '核对经营数据 · 查询被拒绝')
  assert.equal(queryDescription({ success: false, tool_data: { result: { status: 'failed', row_count: 0 } } }), '核对经营数据 · 查询失败')
  assert.equal(queryDescription({ success: true, tool_data: { result: { status: 'ok', row_count: 0 } } }), '核对经营数据 · 0 行结果')
  assert.equal(queryDescription({ success: true, tool_data: { result: { status: 'ok', row_count: 12 } } }), '核对经营数据 · 12 行结果')
  assert.equal(queryDescription({ tool_data: { row_count: 0 } }), '核对经营数据')
})
