import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./MemoryWorkspaceSettings.vue', import.meta.url), 'utf8')

test('disabled runtime configuration explains how the enterprise can enable memory', () => {
  assert.match(source, /v-if="runtime && loaded && !config\.enabled" class="runtime-disabled-state"/)
  assert.match(source, /memoryWorkspaceSettings\.runtimeDisabledTitle/)
  assert.match(source, /memoryWorkspaceSettings\.runtimeDisabledDescription/)
  assert.doesNotMatch(source, /runtime[^\n]*<t-switch[^\n]*config\.enabled/)
})
