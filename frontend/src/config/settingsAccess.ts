export type SettingsRoleKey = 'viewer' | 'admin'

/**
 * Enterprise-scoped settings access policy. Platform sections are declared
 * separately in SYSTEM_ADMIN_SETTINGS_SECTIONS.
 *
 * Keep this as the single frontend source of truth for both the complete
 * Settings navigation and any shortcuts that lead into it. Backend route
 * guards remain authoritative.
 */
export const SETTINGS_SECTION_MIN_ROLE: Record<string, SettingsRoleKey> = {
  general: 'viewer',
  'enterprise-skills': 'admin',
  system: 'viewer',
  userprofile: 'viewer',
  tenant: 'admin',
  members: 'admin',
  businessRoles: 'admin',
  mymemory: 'viewer',
  memory: 'admin',
}

/**
 * Product-surface entry policy for routes that are management/catalog
 * surfaces rather than the employee assistant itself. It deliberately does
 * not govern the APIs: server route guards are authoritative.
 */
export const EMPLOYEE_SURFACE_MIN_ROLE = {
  enterpriseAdministration: 'admin',
  knowledgeBases: 'admin',
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

export const SYSTEM_ADMIN_SETTINGS_SECTIONS = new Set([
  'agents',
  'memory-runtime',
  'diagnostics',
  'models',
  'chathistory',
  'websearch',
  'parser',
  'mcp',
  'vectorstore',
  'storage',
  'sandbox',
  'skills',
  'system-global',
  'runtime-queues',
  'system-audit-log',
])

export type SettingsSurface = 'personal' | 'enterprise' | 'platform'

// Surface placement is independent of API authorization. Reuse the existing
// panels without mixing an administrator's work into their personal settings.
export function settingsSurfaceForSection(section: string): SettingsSurface {
  if (SYSTEM_ADMIN_SETTINGS_SECTIONS.has(section)) return 'platform'
  if (['tenant', 'members', 'businessRoles', 'memory', 'enterprise-skills'].includes(section)
      || section.startsWith('integration-')) return 'enterprise'
  return 'personal'
}
