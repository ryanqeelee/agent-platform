import type { Router } from 'vue-router'

export const DEFAULT_EMPLOYEE_WORKSPACE_PATH = '/platform/creatChat'

/**
 * Accept only a route the current SPA can resolve inside the employee product.
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

export function postLoginDestination(router: Pick<Router, 'resolve'>, raw: unknown, hasValidTenant: boolean) {
  return safeReturnTo(router, raw) || (hasValidTenant ? DEFAULT_EMPLOYEE_WORKSPACE_PATH : '/onboarding/workspace')
}
