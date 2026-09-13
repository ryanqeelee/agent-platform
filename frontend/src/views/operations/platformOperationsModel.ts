import type { EnterpriseActivationPayload, EnterpriseUpdatePayload, OperationsEnterprise } from '@/api/platformOperations'

export const GIB_BYTES = 1024 ** 3
export const PENDING_ACTIVATION_STORAGE_KEY = 'weknora_platform_pending_activation_v1'
export const PENDING_INITIAL_ADMIN_STORAGE_KEY = 'weknora_platform_pending_initial_admin_v1'

export interface EnterpriseFormDraft {
  name: string
  description: string
  status: OperationsEnterprise['status']
  analysis_enabled: boolean
  seats_total: number | null
  storage_quota_gib: number
}

export interface ActivationCommand {
  activationId: string
  idempotencyKey: string
  payload: EnterpriseActivationPayload
}

export interface InitialAdministratorCommand {
  commandId: string
  username: string
  email: string
}

export interface EnterpriseCreationDraft {
  name: string
  description: string
  seats_total: number
  storage_quota_gib: number
  username: string
  email: string
  password: string
}

export function freshEnterpriseCreationDraft(): EnterpriseCreationDraft {
  return {
    name: '',
    description: '',
    seats_total: 1,
    storage_quota_gib: 10,
    username: '',
    email: '',
    password: '',
  }
}

export function bytesToGiB(bytes: number): number {
  return Math.max(0, Number(bytes || 0)) / GIB_BYTES
}

export function gibToBytes(gib: number): number {
  return Math.round(Math.max(0, Number(gib || 0)) * GIB_BYTES)
}

export function enterpriseFormDraft(enterprise: OperationsEnterprise): EnterpriseFormDraft {
  return {
    name: enterprise.name,
    description: enterprise.description,
    status: enterprise.status,
    analysis_enabled: enterprise.analysis_enabled,
    seats_total: enterprise.seats_total,
    storage_quota_gib: bytesToGiB(enterprise.storage_quota),
  }
}

export function enterpriseUpdatePayload(
  draft: Omit<EnterpriseFormDraft, 'seats_total'> & { seats_total?: number | null },
): EnterpriseUpdatePayload {
  return {
    name: draft.name.trim(),
    description: draft.description.trim(),
    status: draft.status,
    analysis_enabled: draft.analysis_enabled,
    seats_total: draft.seats_total == null ? null : Number(draft.seats_total),
    storage_quota: gibToBytes(draft.storage_quota_gib),
  }
}

export function activationPayload(
  draft: Pick<EnterpriseFormDraft, 'name' | 'description' | 'storage_quota_gib'> & { seats_total: number },
  initialAdministratorUserId: string,
): EnterpriseActivationPayload {
  return {
    name: draft.name.trim(),
    description: draft.description.trim(),
    seats_total: Number(draft.seats_total),
    storage_quota: gibToBytes(draft.storage_quota_gib),
    initial_administrator_user_id: initialAdministratorUserId,
  }
}

export function createActivationCommand(
  payload: EnterpriseActivationPayload,
  randomUUID: () => string,
): ActivationCommand {
  return {
    activationId: randomUUID(),
    idempotencyKey: randomUUID(),
    payload,
  }
}

export function initialAdministratorCommand(
  current: InitialAdministratorCommand | null,
  username: string,
  email: string,
  randomUUID: () => string,
): InitialAdministratorCommand {
  const normalized = { username: username.trim(), email: email.trim() }
  if (current && current.username === normalized.username && current.email === normalized.email) return current
  return { commandId: randomUUID(), ...normalized }
}
