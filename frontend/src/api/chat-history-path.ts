import { platformTenantPath } from './platform-tenant-path'

export type PlatformChatHistoryResource = 'chat-history-config' | 'chat-history-stats'

export function platformChatHistoryPath(tenantId: number, resource: PlatformChatHistoryResource): string {
  return platformTenantPath(tenantId, resource)
}
