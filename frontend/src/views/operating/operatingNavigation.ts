const OPERATING_AGENT = 'builtin-operating-analyst'

export function retiredOperatingAnalysisQueryRedirect(query: Record<string, unknown>) {
  return typeof query.data_session === 'string'
    ? { path: '/platform/operating-analysis', replace: true as const }
    : null
}

export function nativeChatPath(item: {
  id: string
  path?: string
  last_request_state?: { agent_id?: string }
}, knownOperating = false): string {
  return item.last_request_state?.agent_id === OPERATING_AGENT || knownOperating
    || item.path?.startsWith('operating-analysis/chat/')
    ? `operating-analysis/chat/${item.id}`
    : `chat/${item.id}`
}

export function operatingMenuTarget(currentPath: string, lastSessionId: string): string {
  return lastSessionId && !currentPath.startsWith('/platform/operating-analysis')
    ? `/platform/operating-analysis/chat/${lastSessionId}`
    : '/platform/operating-analysis'
}
