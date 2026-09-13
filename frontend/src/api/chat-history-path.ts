export type PlatformChatHistoryResource = 'chat-history-config' | 'chat-history-stats'

export function platformChatHistoryPath(resource: PlatformChatHistoryResource): string {
  return `/api/v1/system/admin/${resource}`
}
