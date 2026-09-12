import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const systemAPI = readFileSync(new URL('./system/index.ts', import.meta.url), 'utf8')
const skillAPI = readFileSync(new URL('./skill/index.ts', import.meta.url), 'utf8')
const settings = readFileSync(new URL('../views/settings/Settings.vue', import.meta.url), 'utf8')
const nginx = readFileSync(new URL('../../nginx.conf', import.meta.url), 'utf8')

test('platform sandbox and skill APIs require an explicit tenant path', () => {
  assert.match(systemAPI, /\/api\/v1\/system\/admin\/tenants\/\$\{tenantId\}\/sandbox-configs/)
  assert.match(skillAPI, /\/api\/v1\/system\/admin\/tenants\/\$\{tenantId\}\/skills/)
  assert.doesNotMatch(systemAPI, /['`]\/api\/v1\/sandbox-configs/)
  assert.doesNotMatch(skillAPI, /['`]\/api\/v1\/skills/)
})

test('settings provides the selected tenant only to its existing component tree', () => {
  assert.match(settings, /defineProps<\{ tenantControlId\?: number \}>\(\)/)
  assert.match(settings, /provide\(platformTenantControlIDKey, toRef\(props, 'tenantControlId'\)\)/)
  assert.match(settings, /\(key === 'sandbox' \|\| key === 'skills'\) && !props\.tenantControlId/)
})

test('nginx grants the larger skill bundle limit only to the tenant-scoped control plane', () => {
  assert.ok(nginx.includes('location ~ ^/api/v1/system/admin/tenants/[1-9][0-9]*/(?:skills/catalog|sandbox-configs/[^/]+/skills)/?$ {'))
  assert.ok(!nginx.includes('location ~ ^/api/v1/(?:skills/catalog|sandbox-configs/[^/]+/skills)/?$ {'))
})
