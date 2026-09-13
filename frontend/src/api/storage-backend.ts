import { get, post, put, del } from '@/utils/request'
const basePath = '/api/v1/system/admin/storage-backends'
const capabilityPath = '/api/v1/storage-backends'

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
  name: string
  provider: string
  config: StorageBackendConfig
  source: 'user'
  status: 'active' | 'disabled'
  created_at?: string
  updated_at?: string
}

export interface StorageBackendListResponse {
  success: boolean
  data: StorageBackend[]
  default_storage_backend_id?: string | null
}

export interface StorageBackendCapability {
  id: string
  name: string
  provider: string
  status: 'active'
}

export interface StorageBackendCapabilitiesResponse {
  success: boolean
  data: StorageBackendCapability[]
  default_storage_backend_id?: string | null
}

export const listStorageBackends = (): Promise<StorageBackendListResponse> => get(basePath)
export const listStorageBackendCapabilities = (): Promise<StorageBackendCapabilitiesResponse> => get(capabilityPath)
export const listStorageBackendTypes = (): Promise<{ success: boolean; data: string[] }> => get(`${basePath}/types`)
export const createStorageBackend = (data: Partial<StorageBackend>) => post(basePath, data)
export const updateStorageBackend = (id: string, data: Partial<StorageBackend>) => put(`${basePath}/${id}`, data)
export const deleteStorageBackend = (id: string) => del(`${basePath}/${id}`)
export const setDefaultStorageBackend = (id: string) => put(`${basePath}/${id}/default`, {})
export const testStorageBackend = (data: Partial<StorageBackend>) => post(`${basePath}/test`, data)
export const testStorageBackendByID = (id: string) => post(`${basePath}/${id}/test`, {})
