import { get, patch, post, put } from '@/utils/request'

const base = '/api/v1/system/admin/operations'

interface OperationsResponse<T> {
  success: true
  data: T
}

const unwrapOperationsData = <T>(response: OperationsResponse<T>) => response.data

export interface OperationsEnterprise {
  id: number
  name: string
  description: string
  status: 'active' | 'suspended' | 'provisioning' | 'activation_abandoned'
  seats_total: number | null
  seats_used: number
  storage_quota: number
  storage_used?: number
}

export type EnterpriseUpdatePayload = Pick<
  OperationsEnterprise,
  'name' | 'description' | 'status' | 'seats_total' | 'storage_quota'
>

export interface OperationsMember {
  user_id: string
  username: string
  email: string
  role: 'admin' | 'viewer'
  status: 'active' | 'suspended'
}

export interface EnterpriseActivationPayload {
  name: string
  description: string
  seats_total: number
  storage_quota: number
  initial_administrator_user_id: string
}

export interface EnterpriseActivation {
  schema: string
  activationId: string
  enterpriseId: string
  productBaseTenantId: string | null
  bindingId: string | null
  status: 'pending' | 'pb_prepared' | 'ready' | 'completed' | 'abandoned'
  lastErrorCode: string | null
  name: string
  description: string
  seatsTotal: number
  storageQuota: number
  initialAdministratorUserId: string
  aiCapabilityPlanVersionId: string
  createdAt: string
  updatedAt: string
  completedAt: string | null
}

export interface InitialAdministratorCommandResponse {
  command_id: string
  user_id: string
  username: string
  email: string
  status: 'created'
  replayed: boolean
}

export const listOperationsEnterprises = () =>
  get<OperationsResponse<{ items: OperationsEnterprise[]; total: number }>>(`${base}/enterprises`).then(unwrapOperationsData)

export const getOperationsEnterprise = (tenantId: number) =>
  get<OperationsResponse<OperationsEnterprise>>(`${base}/enterprises/${tenantId}`).then(unwrapOperationsData)

export const updateOperationsEnterprise = (
  tenantId: number,
  payload: EnterpriseUpdatePayload,
) => patch<OperationsResponse<OperationsEnterprise>>(`${base}/enterprises/${tenantId}`, payload).then(unwrapOperationsData)

export interface OperationsMemberPage {
  items: OperationsMember[]
  total: number
  page: number
  page_size: number
}

export const listOperationsMembers = (tenantId: number, page: number, pageSize: number, query = '') => {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
  if (query.trim()) params.set('q', query.trim())
  return get<OperationsResponse<OperationsMemberPage>>(
    `${base}/enterprises/${tenantId}/members?${params.toString()}`,
  ).then(unwrapOperationsData)
}

export const createOperationsEmployee = (tenantId: number, payload: { username: string; email: string; password: string }) =>
  post<OperationsResponse<unknown>>(`${base}/enterprises/${tenantId}/employees`, payload).then(unwrapOperationsData)

export const updateOperationsMemberRole = (tenantId: number, userId: string, role: OperationsMember['role']) =>
  put<OperationsResponse<unknown>>(`${base}/enterprises/${tenantId}/members/${encodeURIComponent(userId)}/role`, { role }).then(unwrapOperationsData)

export const updateOperationsMemberStatus = (tenantId: number, userId: string, status: OperationsMember['status']) =>
  put<OperationsResponse<unknown>>(`${base}/enterprises/${tenantId}/members/${encodeURIComponent(userId)}/status`, { status }).then(unwrapOperationsData)

export const resetOperationsMemberPassword = (tenantId: number, userId: string, newPassword: string) =>
  post<OperationsResponse<unknown>>(`${base}/enterprises/${tenantId}/members/${encodeURIComponent(userId)}/password-reset`, { new_password: newPassword }).then(unwrapOperationsData)

export const createInitialAdministrator = (
  commandId: string,
  payload: { username: string; email: string; password: string },
) => post<OperationsResponse<InitialAdministratorCommandResponse>>(
  `${base}/initial-administrators`,
  payload,
  { headers: { 'Idempotency-Key': commandId } },
).then(unwrapOperationsData)

export const getInitialAdministratorCommand = (commandId: string) =>
  get<OperationsResponse<InitialAdministratorCommandResponse>>(
    `${base}/initial-administrators/${encodeURIComponent(commandId)}`,
  ).then(unwrapOperationsData)

export const getEnterpriseActivation = (activationId: string) =>
  get<OperationsResponse<EnterpriseActivation>>(`${base}/enterprise-activations/${encodeURIComponent(activationId)}`).then(unwrapOperationsData)

export const activateEnterprise = (
  activationId: string,
  idempotencyKey: string,
  payload: EnterpriseActivationPayload,
) => put<OperationsResponse<EnterpriseActivation>>(
  `${base}/enterprise-activations/${encodeURIComponent(activationId)}`,
  payload,
  { headers: { 'Idempotency-Key': idempotencyKey } },
).then(unwrapOperationsData)
