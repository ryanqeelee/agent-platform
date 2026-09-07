import type { TenantRole } from './members'

const tenantRoles: readonly TenantRole[] = ['admin', 'viewer']

export function tenantRoleTranslationKey(value: unknown): string {
  return typeof value === 'string' && tenantRoles.includes(value as TenantRole)
    ? `tenantMember.role.${value}` : ''
}

export function canManageMemberRole(actor: TenantRole | '', target: TenantRole, platformOperator = false, isSelf = false): boolean {
  return tenantRoles.includes(target) && (platformOperator || (actor === 'admin' && !isSelf))
}

export function assignableMemberRoles(actor: TenantRole | '', platformOperator = false): TenantRole[] {
  return platformOperator || actor === 'admin' ? ['admin', 'viewer'] : []
}
