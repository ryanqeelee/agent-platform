export const BUILTIN_EMPLOYEE_ASSISTANT_ID = 'builtin-employee-assistant'

/** Web permission is effective only when the current tenant's server projection is ready. */
export function employeeWebSearchEnabled(permitted: boolean, agent?: {
  web_search_ready?: boolean
  config?: { web_search_enabled?: boolean }
}): boolean {
  return permitted && agent?.config?.web_search_enabled === true && agent?.web_search_ready === true
}
