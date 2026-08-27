export type SettingsRoleKey = 'viewer' | 'contributor' | 'admin' | 'owner'

/**
 * Workspace-scoped settings access policy.
 *
 * Keep this as the single frontend source of truth for both the complete
 * Settings navigation and any shortcuts that lead into it. Backend route
 * guards remain authoritative.
 */
export const SETTINGS_SECTION_MIN_ROLE: Record<string, SettingsRoleKey> = {
  general: 'viewer',
  websearch: 'admin',
  chathistory: 'admin',
  parser: 'admin',
  system: 'viewer',
  userprofile: 'viewer',
  tenant: 'viewer',
  members: 'viewer',
  businessRoles: 'admin',
}

/**
 * Product-surface entry policy for routes that are management/catalog
 * surfaces rather than the employee assistant itself. It deliberately does
 * not govern the APIs: server route guards are authoritative.
 */
export const EMPLOYEE_SURFACE_MIN_ROLE = {
  knowledgeBases: 'contributor',
  agents: 'admin',
  organizations: 'admin',
} as const satisfies Record<string, SettingsRoleKey>

export const employeeSurfaceMinRoleForPath = (path: string): SettingsRoleKey | undefined => {
  if (path === '/platform/agents') return EMPLOYEE_SURFACE_MIN_ROLE.agents
  if (path === '/platform/organizations') return EMPLOYEE_SURFACE_MIN_ROLE.organizations
  if (path === '/platform/knowledge-bases') {
    return EMPLOYEE_SURFACE_MIN_ROLE.knowledgeBases
  }
  return undefined
}

/**
 * A management-labelled avatar shortcut has a stricter threshold than the
 * corresponding read-only Settings page.
 */
export const SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE = {
  members: 'admin',
} as const satisfies Record<string, SettingsRoleKey>

export const SYSTEM_ADMIN_SETTINGS_SECTIONS = new Set([
  'models',
  'chathistory',
  'websearch',
  'parser',
	'mcp',
  'ollama',
  'weknoracloud',
  'vectorstore',
  'storage',
  'system-global',
  'runtime-queues',
  'platform-api-keys',
  'system-audit-log',
])
