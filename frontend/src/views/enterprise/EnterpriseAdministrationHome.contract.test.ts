import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const source = readFileSync(fileURLToPath(new URL('./EnterpriseAdministrationHome.vue', import.meta.url)), 'utf8')

test('enterprise home consumes server-owned health and retains management destinations', () => {
  assert.match(source, /getEnterpriseAdministrationQueue\(\)/)
  assert.match(source, /queue.edge_nodes != null/)
  assert.match(source, /node.availability/)
  assert.match(source, /node.data_service_status/)
  assert.match(source, /router.push\('\/platform\/knowledge-bases'\)/)
  assert.match(source, /path: '\/platform\/settings', query: \{ section \}/)
  assert.doesNotMatch(source, /queue.items|service-health|quotaLabel/)
})
