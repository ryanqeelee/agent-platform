import { get, put, post } from '@/utils/request'
import { platformChatHistoryPath } from './chat-history-path'

// ChatHistoryConfig is the tenantless platform message-index policy.
export interface ChatHistoryConfig {
  enabled: boolean
  embedding_model_id: string
}

// PlatformChatHistoryStats aggregates every tenant-private message-index KB.
export interface PlatformChatHistoryStats {
  enabled: boolean
  embedding_model_id?: string
  tenant_knowledge_base_count: number
  indexed_message_count: number
  has_indexed_messages: boolean
}

// MessageSearchRequest defines search parameters for message search
export interface MessageSearchRequest {
  query: string
  mode?: 'keyword' | 'vector' | 'hybrid'
  limit?: number
  session_ids?: string[]
}

// MessageSearchGroupItem represents a merged Q&A pair in search results
export interface MessageSearchGroupItem {
  request_id: string
  session_id: string
  session_title: string
  query_content: string
  answer_content: string
  score: number
  match_type: string
  created_at: string
}

// MessageSearchResult represents the full search result
export interface MessageSearchResult {
  items: MessageSearchGroupItem[]
  total: number
}

export function getPlatformChatHistoryConfig() {
  return get(platformChatHistoryPath('chat-history-config'))
}

export function updatePlatformChatHistoryConfig(config: ChatHistoryConfig) {
  return put(platformChatHistoryPath('chat-history-config'), config)
}

export function getPlatformChatHistoryStats() {
  return get(platformChatHistoryPath('chat-history-stats'))
}

// Search messages across all sessions (keyword + vector hybrid search)
export function searchMessages(data: MessageSearchRequest) {
  return post('/api/v1/messages/search', data)
}
