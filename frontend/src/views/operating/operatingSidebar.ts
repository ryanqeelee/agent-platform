import type { OperatingLocation, OperatingSnapshot } from './operatingClient'

export function operatingSessionLocation(current: OperatingLocation, id: string): OperatingLocation {
  return {
    ...current,
    surface: 'analysis',
    sessionId: id,
    ...(id === current.sessionId ? {} : { artifactId: null, tab: 'report' as const }),
  }
}

export type OperatingSidebarSession = {
  id: string
  path: string
  title: string
  updated_at: string
}

export type OperatingSidebarProjection = {
  activePath: string
  sessions: OperatingSidebarSession[]
}

export function projectOperatingSidebar(
  snapshot: Pick<OperatingSnapshot, 'location' | 'sessions'> | null,
): OperatingSidebarProjection {
  if (!snapshot) return { activePath: '', sessions: [] }

  return {
    activePath: snapshot.location.sessionId ?? '',
    sessions: snapshot.sessions.map((session) => ({
      id: session.id,
      path: session.id,
      title: session.title || '未命名分析',
      updated_at: session.updatedAt,
    })),
  }
}
