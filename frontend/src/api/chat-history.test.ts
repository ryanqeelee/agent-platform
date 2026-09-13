import assert from 'node:assert/strict'
import test from 'node:test'

import { platformChatHistoryPath } from './chat-history-path'

test('platform chat-history requests are tenantless', () => {
  assert.equal(
    platformChatHistoryPath('chat-history-config'),
    '/api/v1/system/admin/chat-history-config',
  )
  assert.equal(
    platformChatHistoryPath('chat-history-stats'),
    '/api/v1/system/admin/chat-history-stats',
  )
})
