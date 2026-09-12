import type { Router } from 'vue-router'

export const DEFAULT_EMPLOYEE_WORKSPACE_PATH = '/home'
export const ENTERPRISE_KNOWLEDGE_BASE_PATH = '/platform/knowledge-bases'
export const PLATFORM_OPERATIONS_PATH = '/platform/operations'
export const WORKSPACE_ONBOARDING_PATH = '/onboarding/workspace'

export function defaultAuthenticatedDestination(hasValidTenant: boolean, isSystemAdmin: boolean): string {
  if (isSystemAdmin) return PLATFORM_OPERATIONS_PATH
  return hasValidTenant ? DEFAULT_EMPLOYEE_WORKSPACE_PATH : WORKSPACE_ONBOARDING_PATH
}

export function oidcInvitationDestination(isSystemAdmin: boolean): string {
  return isSystemAdmin ? PLATFORM_OPERATIONS_PATH : ENTERPRISE_KNOWLEDGE_BASE_PATH
}

export function tenantRequiredRouteFallback(
  hasValidTenant: boolean,
  isSystemAdmin: boolean,
): string | null {
  if (isSystemAdmin) return PLATFORM_OPERATIONS_PATH
  if (!hasValidTenant) return WORKSPACE_ONBOARDING_PATH
  return null
}

/**
 * Accept only an authenticated route the current SPA can resolve.
 * The destination remains in the URL for one login round-trip only; callers
 * must not persist it in browser storage.
 */
export function safeReturnTo(router: Pick<Router, 'resolve'>, raw: unknown): string | null {
  if (typeof raw !== 'string' || raw.length === 0) return null
  if (
    !raw.startsWith('/platform/') ||
    raw.startsWith('//') ||
    raw.includes('\\') ||
    /[\u0000-\u001f\u007f]/.test(raw)
  ) {
    return null
  }

  const resolved = router.resolve(raw)
  if (
    resolved.matched.length === 0 ||
    !resolved.path.startsWith('/platform/') ||
    resolved.path === '/login' ||
    resolved.path === '/register' ||
    resolved.path.startsWith('/onboarding/')
  ) {
    return null
  }
  return raw
}

export function loginDestination(router: Pick<Router, 'resolve'>, raw: unknown) {
  const returnTo = safeReturnTo(router, raw)
  return returnTo
    ? { path: '/login', query: { returnTo } }
    : { path: '/login' }
}

export function postLoginDestination(
  router: Pick<Router, 'resolve'>,
  raw: unknown,
  hasValidTenant: boolean,
  isSystemAdmin: boolean,
) {
  if (isSystemAdmin) return PLATFORM_OPERATIONS_PATH
  return safeReturnTo(router, raw) || defaultAuthenticatedDestination(hasValidTenant, isSystemAdmin)
}
