import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./MemoryWorkspaceSettings.vue', import.meta.url), 'utf8')

test('platform runtime configuration is tenantless and independent of enterprise consent', () => {
  assert.match(source, /getPlatformMemoryRuntimeConfig\(\)/)
  assert.match(source, /updatePlatformMemoryRuntimeConfig\(runtimeConfig\)/)
  assert.doesNotMatch(source, /usePlatformTenantControlID/)
  assert.doesNotMatch(source, /v-if="runtime && config\.enabled"/)
  assert.doesNotMatch(source, /runtimeDisabledTitle|runtimeDisabledDescription/)
})
