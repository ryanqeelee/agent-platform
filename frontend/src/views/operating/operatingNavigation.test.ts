import assert from 'node:assert/strict'
import test from 'node:test'
import {
  nativeChatPath,
  operatingMenuTarget,
  retiredOperatingAnalysisQueryRedirect,
} from './operatingNavigation.ts'

test('session-list grouping retains operating origin before and after first request state', () => {
  assert.equal(nativeChatPath({ id: 'a', last_request_state: { agent_id: 'builtin-operating-analyst' } }), 'operating-analysis/chat/a')
  assert.equal(nativeChatPath({ id: 'a', path: 'operating-analysis/chat/a' }), 'operating-analysis/chat/a')
  assert.equal(nativeChatPath({ id: 'a' }, true), 'operating-analysis/chat/a')
  assert.equal(nativeChatPath({ id: 'employee' }), 'chat/employee')
})

test('return from another page opens latest analysis; a deliberate new analysis stays available', () => {
  assert.equal(operatingMenuTarget('/platform/knowledge-bases', 'a'), '/platform/operating-analysis/chat/a')
  assert.equal(operatingMenuTarget('/platform/operating-brief', 'a'), '/platform/operating-analysis/chat/a')
  assert.equal(operatingMenuTarget('/platform/operating-analysis/chat/a', 'a'), '/platform/operating-analysis')
  assert.equal(operatingMenuTarget('/platform/knowledge-bases', ''), '/platform/operating-analysis')
})

test('retired Center session query returns to the native analysis home', () => {
  assert.deepEqual(retiredOperatingAnalysisQueryRedirect({ data_session: 'legacy/session' }), {
    path: '/platform/operating-analysis',
    replace: true,
  })
  assert.equal(retiredOperatingAnalysisQueryRedirect({}), null)
})
