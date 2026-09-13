import { get, post, put, del } from '@/utils/request'
import { platformTenantPath } from './platform-tenant-path'

const basePath = (tenantId?: number) => tenantId === undefined ? '/api/v1/web-search-providers' : platformTenantPath(tenantId, 'web-search-providers')

// WebSearchProviderEntity represents a configured web search provider instance
export interface WebSearchProviderEntity {
  id?: string
  tenant_id?: number
  name: string
  provider: 'bing' | 'google' | 'duckduckgo' | 'tavily' | 'ollama' | 'baidu' | 'searxng' | 'keenable' | 'zhipu' | 'metaso' | 'exa'
  description?: string
  parameters: {
    // api_key is never returned by the server in this shape; it lives behind
    // the /credentials subresource. Kept on the type so the initial create
    // POST can still include it.
    api_key?: string
    engine_id?: string
    base_url?: string
    proxy_url?: string
    extra_config?: Record<string, string>
  }
  is_default?: boolean
  // Per-field configured? metadata from the main response.
  credentials?: Record<WebSearchCredentialField, { configured: boolean }>
  created_at?: string
  updated_at?: string
}

// WebSearchProviderTypeInfo describes metadata for a provider type
export interface WebSearchProviderTypeInfo {
  id: string
  name: string
  requires_api_key: boolean
  // Keyless-by-default providers that still accept an optional key (e.g. Keenable).
  supports_optional_api_key?: boolean
  requires_engine_id?: boolean
  requires_base_url?: boolean
  supports_proxy?: boolean
  description?: string
  docs_url?: string
  config_fields?: WebSearchProviderConfigField[]
}

export interface WebSearchProviderConfigField {
  key: string
  label: string
  label_key?: string
  type: 'select'
  required?: boolean
  default?: string
  description?: string
  description_key?: string
  options?: Array<{ label: string; label_key?: string; value: string }>
}

// Create a new web search provider
export function createWebSearchProvider(data: Partial<WebSearchProviderEntity>, tenantId?: number) {
  return post(basePath(tenantId), data)
}

// List all web search providers for the current tenant
export function listWebSearchProviders(tenantId?: number) {
  return get(basePath(tenantId))
}

// Get a single web search provider by ID
export function getWebSearchProvider(id: string, tenantId?: number) {
  return get(`${basePath(tenantId)}/${id}`)
}

// Update an existing web search provider
export function updateWebSearchProvider(id: string, data: Partial<WebSearchProviderEntity>, tenantId?: number) {
  return put(`${basePath(tenantId)}/${id}`, data)
}

// Delete a web search provider
export function deleteWebSearchProvider(id: string, tenantId?: number) {
  return del(`${basePath(tenantId)}/${id}`)
}

// Get available provider types (for dynamic form rendering)
export function listWebSearchProviderTypes(tenantId?: number): Promise<WebSearchProviderTypeInfo[]> {
  return get(`${basePath(tenantId)}/types`).then((res: any) => {
    if (res.success && res.data) {
      return res.data
    }
    return []
  })
}

// ----------------------------------------------------------------------------
// Web search provider credential subresource.
// ----------------------------------------------------------------------------

export type WebSearchCredentialField = 'api_key'

export interface WebSearchCredentialsResponse {
  fields: Record<WebSearchCredentialField, { configured: boolean }>
}

export async function putWebSearchProviderCredentials(
  id: string,
  body: Partial<Record<WebSearchCredentialField, string>>,
  tenantId?: number,
): Promise<WebSearchCredentialsResponse> {
  const response: any = await put(`${basePath(tenantId)}/${id}/credentials`, body)
  return (response.data ?? response) as WebSearchCredentialsResponse
}

export async function deleteWebSearchProviderCredentialField(
  id: string,
  field: WebSearchCredentialField,
  tenantId?: number,
): Promise<void> {
  await del(`${basePath(tenantId)}/${id}/credentials/${field}`)
}

// Test a web search provider connection.
// If id is provided, tests the existing saved provider.
// If data is provided, tests with raw credentials (no persistence).
export function testWebSearchProvider(id?: string, data?: { provider: string; parameters: any }, tenantId?: number): Promise<any> {
  if (id) {
    return post(`${basePath(tenantId)}/${id}/test`, {})
  }
  return post(`${basePath(tenantId)}/test`, data || {})
}
