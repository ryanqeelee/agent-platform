import { get, post, put, del } from '@/utils/request'
import { platformTenantPath } from './platform-tenant-path'
const basePath = (tenantId?: number) => tenantId === undefined ? '/api/v1/storage-backends' : platformTenantPath(tenantId, 'storage-backends')

export interface StorageBackendConfig {
  mode?: string
  endpoint?: string
  region?: string
  access_key_id?: string
  secret_access_key?: string
  bucket_name?: string
  path_prefix?: string
  app_id?: string
  use_ssl?: boolean
  force_path_style?: boolean
  use_temp_bucket?: boolean
  temp_bucket_name?: string
  temp_region?: string
}

export interface StorageBackend {
  id: string
  tenant_id?: number
  name: string
  provider: string
  config: StorageBackendConfig
  source: 'user' | 'env'
  status: 'active' | 'disabled'
  legacy_alias?: boolean
  created_at?: string
  updated_at?: string
}

export interface StorageBackendListResponse {
  success: boolean
  data: StorageBackend[]
  default_storage_backend_id?: string | null
}

export const listStorageBackends = (tenantId?: number): Promise<StorageBackendListResponse> => get(basePath(tenantId))
export const listStorageBackendTypes = (tenantId?: number): Promise<{ success: boolean; data: string[] }> => get(`${basePath(tenantId)}/types`)
export const createStorageBackend = (data: Partial<StorageBackend>, tenantId?: number) => post(basePath(tenantId), data)
export const updateStorageBackend = (id: string, data: Partial<StorageBackend>, tenantId?: number) => put(`${basePath(tenantId)}/${id}`, data)
export const deleteStorageBackend = (id: string, tenantId?: number) => del(`${basePath(tenantId)}/${id}`)
export const setDefaultStorageBackend = (id: string, tenantId?: number) => put(`${basePath(tenantId)}/${id}/default`, {})
export const testStorageBackend = (data: Partial<StorageBackend>, tenantId?: number) => post(`${basePath(tenantId)}/test`, data)
export const testStorageBackendByID = (id: string, tenantId?: number) => post(`${basePath(tenantId)}/${id}/test`, {})
