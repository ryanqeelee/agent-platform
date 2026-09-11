type QueryEvent = {
  pending?: boolean
  success?: boolean
  tool_data?: { contract_version?: string; query?: { rows_returned?: number }; intent?: string; status?: string; row_count?: number; result?: { status?: string; row_count?: number } }
}

export function queryDescription(event: QueryEvent): string {
  const intent = event.tool_data?.intent || '核对经营数据'
  if (event.pending) return `${intent}…`
  const edgeRows = event.tool_data?.contract_version === 'edge-governed-query-v1' ? event.tool_data.query?.rows_returned : undefined
  if (event.success === true && typeof edgeRows === 'number') return `${intent} · ${edgeRows} 行结果`
  const result = event.tool_data?.result || event.tool_data
  if (result?.status === 'rejected') return `${intent} · 查询被拒绝`
  if (result?.status === 'cancelled') return `${intent} · 查询已取消`
  if (event.success === false || result?.status === 'failed' || result?.status === 'error') return `${intent} · 查询失败`
  const succeeded = event.success === true && (result?.status === 'ok' || result?.status === 'succeeded')
  return `${intent}${succeeded && typeof result?.row_count === 'number' ? ` · ${result.row_count} 行结果` : ''}`
}
