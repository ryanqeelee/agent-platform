import type { OperatingLocation } from './operatingClient'

const MODES = ['auto', 'quick', 'deep', 'extract'] as const
const ARTIFACT_TABS = ['report', 'chart', 'table', 'caliber'] as const
const NAVIGATION_KEYS = [
  'data_surface',
  'data_session',
  'data_mode',
  'data_artifact',
  'data_tab',
] as const

type OperatingMode = typeof MODES[number]
type OperatingArtifactTab = typeof ARTIFACT_TABS[number]
type RouteQuery = Record<string, unknown>

export type OperatingNavigationState = {
  data_surface: 'brief' | 'analysis'
  data_session?: string
  data_mode?: OperatingMode
  data_artifact?: string
  data_tab?: OperatingArtifactTab
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value)

const hasOnlyKeys = (value: Record<string, unknown>, allowed: readonly string[]) =>
  Object.keys(value).every((key) => allowed.includes(key))

const isOneOf = <T extends string>(value: unknown, values: readonly T[]): value is T =>
  typeof value === 'string' && values.includes(value as T)

const isBoundedNavigationId = (value: unknown): value is string =>
  typeof value === 'string'
  && value.length > 0
  && value.length <= 128
  && /^[A-Za-z0-9][A-Za-z0-9._:-]*$/.test(value)

const singleQueryValue = (value: unknown): string | undefined =>
  typeof value === 'string' ? value : undefined

export function isOperatingRoutePath(path: string): boolean {
  return path === '/platform/operating-brief' || path === '/platform/operating-analysis'
}

export function surfaceForOperatingPath(path: string): 'brief' | 'analysis' {
  return path === '/platform/operating-brief' ? 'brief' : 'analysis'
}

export function parseOperatingNavigationState(value: unknown): OperatingNavigationState | null {
  if (!isRecord(value) || !hasOnlyKeys(value, NAVIGATION_KEYS)) return null
  if (value.data_surface !== 'brief' && value.data_surface !== 'analysis') return null
  if (value.data_session !== undefined && !isBoundedNavigationId(value.data_session)) return null
  if (value.data_artifact !== undefined && !isBoundedNavigationId(value.data_artifact)) return null
  if (value.data_mode !== undefined && !isOneOf(value.data_mode, MODES)) return null
  if (value.data_tab !== undefined && !isOneOf(value.data_tab, ARTIFACT_TABS)) return null
  return value as OperatingNavigationState
}

export function operatingNavigationStateFromRoute(path: string, query: RouteQuery): OperatingNavigationState {
  const state: OperatingNavigationState = { data_surface: surfaceForOperatingPath(path) }
  const sessionId = singleQueryValue(query.data_session)
  const artifactId = singleQueryValue(query.data_artifact)
  const mode = singleQueryValue(query.data_mode)
  const tab = singleQueryValue(query.data_tab)
  if (isBoundedNavigationId(sessionId)) state.data_session = sessionId
  if (isBoundedNavigationId(artifactId)) state.data_artifact = artifactId
  if (isOneOf(mode, MODES)) state.data_mode = mode
  if (isOneOf(tab, ARTIFACT_TABS)) state.data_tab = tab
  return state
}

export function operatingLocationFromRoute(path: string, query: RouteQuery): OperatingLocation {
  const state = operatingNavigationStateFromRoute(path, query)
  return {
    surface: state.data_surface,
    sessionId: state.data_session ?? null,
    artifactId: state.data_artifact ?? null,
    mode: state.data_mode ?? 'auto',
    tab: state.data_tab ?? 'report',
  }
}

export function operatingRouteLocation(state: OperatingNavigationState): {
  path: string
  query: Record<string, string>
} {
  const query: Record<string, string> = {}
  for (const key of NAVIGATION_KEYS) {
    const value = state[key]
    if (value !== undefined) query[key] = value
  }
  return {
    path: state.data_surface === 'brief'
      ? '/platform/operating-brief'
      : '/platform/operating-analysis',
    query,
  }
}

export function operatingRouteLocationFromRuntime(location: OperatingLocation): {
  path: string
  query: Record<string, string>
} | null {
  const state = parseOperatingNavigationState({
    data_surface: location.surface,
    ...(location.sessionId !== null ? { data_session: location.sessionId } : {}),
    data_mode: location.mode,
    ...(location.artifactId !== null ? { data_artifact: location.artifactId } : {}),
    data_tab: location.tab,
  })
  return state ? operatingRouteLocation(state) : null
}

const sameRouteLocation = (
  a: { path: string; query: Record<string, string> },
  b: { path: string; query: Record<string, string> },
) => a.path === b.path
  && Object.keys(a.query).length === Object.keys(b.query).length
  && Object.entries(a.query).every(([key, value]) => b.query[key] === value)

export function runtimeNavigationTarget(
  active: boolean,
  currentPath: string,
  currentQuery: RouteQuery,
  next: OperatingLocation,
): { path: string; query: Record<string, string> } | null {
  if (!active) return null
  const target = operatingRouteLocationFromRuntime(next)
  if (!target) return null
  const current = normalizedOperatingRoute(currentPath, currentQuery)
  return sameRouteLocation(current, target) ? null : target
}

export function operatingLocationKey(location: OperatingLocation): string {
  return [
    location.surface,
    location.sessionId ?? '',
    location.artifactId ?? '',
    location.mode,
    location.tab,
  ].join('\0')
}

export function hasOperatingNavigationContext(state: OperatingNavigationState): boolean {
  return NAVIGATION_KEYS.some((key) => key !== 'data_surface' && state[key] !== undefined)
}

export function shouldRestoreOperatingLocation(
  firstActivation: boolean,
  handoffPending: boolean,
  requested: OperatingNavigationState,
  previous: OperatingLocation,
): boolean {
  if (!firstActivation || handoffPending || hasOperatingNavigationContext(requested)) return false
  return previous.sessionId !== null || previous.artifactId !== null
    || previous.mode !== 'auto' || previous.tab !== 'report'
}

export function normalizedOperatingRoute(path: string, query: RouteQuery) {
  return operatingRouteLocation(operatingNavigationStateFromRoute(path, query))
}

export function operatingRouteIsCanonical(path: string, query: RouteQuery): boolean {
  const normalized = normalizedOperatingRoute(path, query)
  if (normalized.path !== path) return false
  const incomingKeys = Object.keys(query)
  const normalizedKeys = Object.keys(normalized.query)
  return incomingKeys.length === normalizedKeys.length
    && normalizedKeys.every((key) => query[key] === normalized.query[key])
}
