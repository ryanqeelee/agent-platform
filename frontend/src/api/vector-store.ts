import { get, post, put, del } from '@/utils/request'
const basePath = '/api/v1/system/admin/vector-stores'
const capabilityPath = '/api/v1/vector-stores'

// ===== Types =====

export interface VectorStoreEntity {
  id?: string
  name: string
  engine_type: string
  connection_config: Record<string, any>
  index_config: Record<string, any>
  created_at?: string
  updated_at?: string
}

export interface VectorStoreCapability {
  id: string
  name: string
  engine_type: string
}

export interface VectorStoreCapabilitiesResponse {
  success: boolean
  data: VectorStoreCapability[]
  default_vector_store_id?: string | null
}

export interface VectorStoreTypeInfo {
  type: string
  display_name: string
  connection_fields: FieldSchema[]
  index_fields: FieldSchema[]
}

export interface FieldSchema {
  name: string
  type: 'string' | 'number' | 'boolean'
  required: boolean
  sensitive?: boolean
  description?: string
  default?: any
  // Inclusive bounds for number fields (omitempty on the backend). When
  // absent the UI falls back to per-field heuristics (isReplicaField).
  min?: number
  max?: number
  // Closed value set for string fields (e.g. knn_engine ∈ lucene|faiss).
  // When non-empty the UI renders a select instead of a free-text input.
  enum?: string[]
  // Marks a field that cannot change after store creation. Informational
  // for now (edit mode is fully read-only); kept for forward use.
  immutable?: boolean
}

// ===== API Functions =====

export function listVectorStoreTypes(): Promise<VectorStoreTypeInfo[]> {
  return get(`${basePath}/types`).then((res: any) => {
    return res.success && res.data ? res.data : []
  })
}

export function listVectorStores(): Promise<{ success: boolean; data: VectorStoreEntity[]; default_vector_store_id?: string | null }> {
  return get(basePath)
}

export function listVectorStoreCapabilities(): Promise<VectorStoreCapabilitiesResponse> { return get(capabilityPath) }

export function createVectorStore(data: Partial<VectorStoreEntity>) {
  return post(basePath, data)
}

export function updateVectorStore(id: string, data: Partial<VectorStoreEntity>) {
  return put(`${basePath}/${id}`, data)
}

export function deleteVectorStore(id: string) {
  return del(`${basePath}/${id}`)
}

export function testVectorStoreRaw(data: { engine_type: string; connection_config: any }): Promise<any> {
  return post(`${basePath}/test`, data)
}

export function testVectorStoreById(id: string): Promise<any> {
  return post(`${basePath}/${id}/test`, {})
}

export function setDefaultVectorStore(id: string) { return put(`${basePath}/${id}/default`, {}) }
