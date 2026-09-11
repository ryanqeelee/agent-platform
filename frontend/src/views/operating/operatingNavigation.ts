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

export interface OperatingPromptOwner { actorId: string; tenantId: string }

export function sameOperatingPromptOwner(a: OperatingPromptOwner, b: OperatingPromptOwner): boolean {
  return Boolean(a.actorId && a.tenantId && a.actorId === b.actorId && a.tenantId === b.tenantId)
}

export function ownedOperatingPromptQuestion(raw: string, owner: OperatingPromptOwner): string | null {
  try {
    const handoff = JSON.parse(raw)
    return handoff?.schema === 'OperatingAnalysisHandoffV1'
      && handoff.owner && sameOperatingPromptOwner(handoff.owner, owner)
      && typeof handoff.question === 'string' && handoff.question.trim()
      ? handoff.question.trim() : null
  } catch {
    return null
  }
}
