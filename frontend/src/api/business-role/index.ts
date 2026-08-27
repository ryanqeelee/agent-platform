import { get, post, put } from '@/utils/request'

export interface BusinessRole {
  id: string
  tenant_id: number
  name: string
  enabled: boolean
}

export interface KnowledgeAccessScope {
  mode: 'all' | 'roles'
  role_ids: string[]
}

export const listBusinessRoles = () => get('/api/v1/business-roles') as Promise<{ data?: BusinessRole[] }>
export const createBusinessRole = (name: string) => post('/api/v1/business-roles', { name })
export const updateBusinessRole = (id: string, name: string, enabled: boolean) => put(`/api/v1/business-roles/${id}`, { name, enabled })
export const getKnowledgeAccessScope = (kbId: string) => get(`/api/v1/knowledge-bases/${kbId}/access`) as Promise<{ data?: KnowledgeAccessScope }>
export const updateKnowledgeAccessScope = (kbId: string, scope: KnowledgeAccessScope) => put(`/api/v1/knowledge-bases/${kbId}/access`, scope)
export const updateMemberBusinessRoles = (tenantId: number, userId: string, roleIDs: string[]) =>
  put(`/api/v1/tenants/${tenantId}/members/${userId}/business-roles`, { role_ids: roleIDs })
