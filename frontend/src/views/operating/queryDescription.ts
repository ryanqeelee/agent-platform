type QueryEvent = {
  pending?: boolean
  success?: boolean
  tool_data?: { intent?: string; status?: string; row_count?: number; result?: { status?: string; row_count?: number } }
}

export function queryDescription(event: QueryEvent): string {
  const intent = event.tool_data?.intent || '核对经营数据'
  if (event.pending) return `${intent}…`
  const result = event.tool_data?.result || event.tool_data
  if (result?.status === 'rejected') return `${intent} · 查询被拒绝`
  if (result?.status === 'cancelled') return `${intent} · 查询已取消`
  if (event.success === false || result?.status === 'failed' || result?.status === 'error') return `${intent} · 查询失败`
  const succeeded = event.success === true && (result?.status === 'ok' || result?.status === 'succeeded')
  return `${intent}${succeeded && typeof result?.row_count === 'number' ? ` · ${result.row_count} 行结果` : ''}`
}
