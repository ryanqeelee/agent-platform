import assert from 'node:assert/strict'
import test from 'node:test'

import { platformChatHistoryPath } from './chat-history-path'

test('platform chat-history requests map to the explicitly selected enterprise', () => {
  assert.equal(
    platformChatHistoryPath(42, 'chat-history-config'),
    '/api/v1/system/admin/tenants/42/chat-history-config',
  )
  assert.equal(
    platformChatHistoryPath(77, 'chat-history-stats'),
    '/api/v1/system/admin/tenants/77/chat-history-stats',
  )
  assert.throws(() => platformChatHistoryPath(0, 'chat-history-config'), /请先选择企业/)
})
