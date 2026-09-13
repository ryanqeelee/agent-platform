import { get, put } from '@/utils/request'

// RetrievalConfig represents the global retrieval/search configuration for a tenant.
// Shared by knowledge search and message search.
export interface RetrievalConfig {
  embedding_top_k: number
  vector_threshold: number
  keyword_threshold: number
  rerank_top_k: number
  rerank_threshold: number
  rerank_model_id: string
}

export interface PlatformRetrievalProcessingSettings {
  contract_version: 'PlatformRetrievalProcessingSettingsV1'
  scope: {
    kind: 'platform_shared' | 'enterprise_assigned'
    product_base_tenant_id: string
  }
  active_plan: {
    contract_version: 'AICapabilityPlanV1'
    version_id: string
  }
  capability_refs: {
    embedding: string
    reranking: string
    parsing: string
  }
}

export async function getPlatformRetrievalProcessingSettings(): Promise<PlatformRetrievalProcessingSettings> {
  const response: any = await get('/api/v1/platform/retrieval-processing-settings')
  if (!response?.success || !response.data) throw new Error('platform retrieval context unavailable')
  return response.data as PlatformRetrievalProcessingSettings
}

// Get tenant retrieval config via KV API
export function getTenantRetrievalConfig() {
  return get('/api/v1/tenants/kv/retrieval-config')
}

// Update tenant retrieval config via KV API
export function updateTenantRetrievalConfig(config: RetrievalConfig) {
  return put('/api/v1/tenants/kv/retrieval-config', config)
}
