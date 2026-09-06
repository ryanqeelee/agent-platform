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
  storage: 'admin',
  sandbox: 'admin',
  // Install writes a root shell into the sandbox image every session of
  // that config boots. Same Admin+ bar as the sandbox editor itself.
  skills: 'admin',
  mcp: 'admin',
  system: 'viewer',
  userprofile: 'viewer',
  tenant: 'viewer',
  members: 'viewer',
  businessRoles: 'admin',
  mymemory: 'viewer',
  memory: 'admin',
  // Every member fills in their own environment variables; the workspace-wide
  // values stay on the Admin+ skills page.
  envvars: 'viewer',
}

/**
 * Product-surface entry policy for routes that are management/catalog
 * surfaces rather than the employee assistant itself. It deliberately does
 * not govern the APIs: server route guards are authoritative.
 */
export const EMPLOYEE_SURFACE_MIN_ROLE = {
  enterpriseAdministration: 'contributor',
  knowledgeBases: 'contributor',
  agents: 'admin',
  organizations: 'admin',
} as const satisfies Record<string, SettingsRoleKey>

export const employeeSurfaceMinRoleForPath = (path: string): SettingsRoleKey | undefined => {
  if (path === '/platform/enterprise') return EMPLOYEE_SURFACE_MIN_ROLE.enterpriseAdministration
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
  models: 'admin',
  skills: 'admin',
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
  'sandbox',
  'skills',
  'system-global',
  'runtime-queues',
  'platform-api-keys',
  'system-audit-log',
])
