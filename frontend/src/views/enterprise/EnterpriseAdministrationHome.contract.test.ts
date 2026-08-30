import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const source = readFileSync(fileURLToPath(new URL('./EnterpriseAdministrationHome.vue', import.meta.url)), 'utf8')

test('enterprise administration uses the governed read-only queue and fixed product targets', () => {
  assert.match(source, /getEnterpriseAdministrationQueue\(\)/)
  assert.match(source, /target === 'service_health'/)
  assert.match(source, /target === 'knowledge'/)
  assert.match(source, /target === 'audit' \? \{ section: 'members', audit: '1' \}/)
})
