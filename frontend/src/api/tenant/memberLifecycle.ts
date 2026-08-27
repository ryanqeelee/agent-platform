import type { TenantMemberStatus, TenantRole } from './members'

const tenantRoles: readonly TenantRole[] = ['owner', 'admin', 'contributor', 'viewer']

/** Returns a product-label translation key without exposing storage role names. */
export function tenantRoleTranslationKey(value: unknown): string {
  return typeof value === 'string' && tenantRoles.includes(value as TenantRole)
    ? `tenantMember.role.${value}`
    : ''
}

/** Mirrors the server membership lifecycle matrix for UI visibility only. */
export function canManageMemberRole(
  actor: TenantRole | '',
  target: TenantRole,
  platformOperator = false,
  isSelf = false,
): boolean {
  if (platformOperator) return target !== 'owner'
  if (isSelf) return false
  if (actor === 'owner') return target !== 'owner'
  if (actor === 'admin') return target === 'contributor' || target === 'viewer'
  return false
}

/** Owner is reserved for the explicit transfer endpoint, never a select option. */
export function assignableMemberRoles(
  actor: TenantRole | '',
  platformOperator = false,
): TenantRole[] {
  if (platformOperator) return ['admin', 'contributor', 'viewer']
  if (actor === 'owner') return ['admin', 'contributor', 'viewer']
  if (actor === 'admin') return ['contributor', 'viewer']
  return []
}

/** Ownership transfer remains a human Owner-only, active-Admin operation. */
export function canTransferMemberOwnership(
  actor: TenantRole | '',
  target: TenantRole,
  targetStatus: TenantMemberStatus,
  isSelf: boolean,
): boolean {
  return actor === 'owner' && target === 'admin' && targetStatus === 'active' && !isSelf
}
