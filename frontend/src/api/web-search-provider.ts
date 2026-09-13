import { get, post, put, del } from '@/utils/request'
const runtimeBasePath = '/api/v1/web-search-providers'
const adminBasePath = '/api/v1/system/admin/web-search-providers'

// WebSearchProviderEntity represents a configured web search provider instance
export interface WebSearchProviderEntity {
  id?: string
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
export function createWebSearchProvider(data: Partial<WebSearchProviderEntity>) {
  return post(adminBasePath, data)
}

// List the enabled global provider catalog available to enterprise runtime.
export function listWebSearchProviders() {
  return get(runtimeBasePath)
}

export function listPlatformWebSearchProviders() {
  return get(adminBasePath)
}

// Get a single web search provider by ID
export function getWebSearchProvider(id: string) {
  return get(`${adminBasePath}/${id}`)
}

// Update an existing web search provider
export function updateWebSearchProvider(id: string, data: Partial<WebSearchProviderEntity>) {
  return put(`${adminBasePath}/${id}`, data)
}

// Delete a web search provider
export function deleteWebSearchProvider(id: string) {
  return del(`${adminBasePath}/${id}`)
}

// Get available provider types (for dynamic form rendering)
export function listWebSearchProviderTypes(): Promise<WebSearchProviderTypeInfo[]> {
  return get(`${adminBasePath}/types`).then((res: any) => {
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
): Promise<WebSearchCredentialsResponse> {
  const response: any = await put(`${adminBasePath}/${id}/credentials`, body)
  return (response.data ?? response) as WebSearchCredentialsResponse
}

export async function deleteWebSearchProviderCredentialField(
  id: string,
  field: WebSearchCredentialField,
): Promise<void> {
  await del(`${adminBasePath}/${id}/credentials/${field}`)
}

// Test a web search provider connection.
// If id is provided, tests the existing saved provider.
// If data is provided, tests with raw credentials (no persistence).
export function testWebSearchProvider(id?: string, data?: { provider: string; parameters: any }): Promise<any> {
  if (id) {
    return post(`${adminBasePath}/${id}/test`, {})
  }
  return post(`${adminBasePath}/test`, data || {})
}
