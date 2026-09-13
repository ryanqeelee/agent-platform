import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const systemAPI = readFileSync(new URL('./system/index.ts', import.meta.url), 'utf8')
const deploymentCapabilitiesStore = readFileSync(new URL('../stores/deploymentCapabilities.ts', import.meta.url), 'utf8')
const skillAPI = readFileSync(new URL('./skill/index.ts', import.meta.url), 'utf8')
const chatHistoryAPI = readFileSync(new URL('./chat-history.ts', import.meta.url), 'utf8')
const chatHistoryPath = readFileSync(new URL('./chat-history-path.ts', import.meta.url), 'utf8')
const platformTenantPath = readFileSync(new URL('./platform-tenant-path.ts', import.meta.url), 'utf8')
const settings = readFileSync(new URL('../views/settings/Settings.vue', import.meta.url), 'utf8')
const chatHistorySettings = readFileSync(new URL('../views/settings/ChatHistorySettings.vue', import.meta.url), 'utf8')
const platformOperations = readFileSync(new URL('../views/operations/PlatformOperations.vue', import.meta.url), 'utf8')
const nginx = readFileSync(new URL('../../nginx.conf', import.meta.url), 'utf8')

test('platform sandbox and skill APIs require an explicit tenant path', () => {
  assert.match(systemAPI, /\/api\/v1\/system\/admin\/tenants\/\$\{tenantId\}\/sandbox-configs/)
  assert.match(skillAPI, /\/api\/v1\/system\/admin\/tenants\/\$\{tenantId\}\/skills/)
  assert.doesNotMatch(systemAPI, /['`]\/api\/v1\/sandbox-configs/)
  assert.doesNotMatch(skillAPI, /['`]\/api\/v1\/skills/)
})

test('deployment capabilities use the tenantless system-admin route for platform login', () => {
  assert.match(systemAPI, /getDeploymentCapabilities\(systemAdmin = false\)/)
  assert.match(systemAPI, /systemAdmin \? '\/api\/v1\/system\/admin\/capabilities' : '\/api\/v1\/system\/capabilities'/)
  assert.match(deploymentCapabilitiesStore, /getDeploymentCapabilities\(useAuthStore\(\)\.isSystemAdmin\)/)
})

test('settings provides the selected tenant only to its existing component tree', () => {
  assert.match(settings, /defineProps<\{ tenantControlId\?: number; tenantControlName\?: string \}>\(\)/)
  assert.match(settings, /provide\(platformTenantControlIDKey, toRef\(props, 'tenantControlId'\)\)/)
  assert.match(settings, /'chathistory'[\s\S]{0,160}!props\.tenantControlId/)
  assert.match(settings, /<ChatHistorySettings :key="props\.tenantControlId"/)
  assert.match(settings, /props\.tenantControlName \|\| authStore\.currentTenantName/)
})

test('message indexing uses the selected enterprise control-plane path', () => {
  assert.match(chatHistoryAPI, /platformChatHistoryPath\(platformTenantId, 'chat-history-config'\)/)
  assert.match(chatHistoryPath, /platformTenantPath\(tenantId, resource\)/)
  assert.match(platformTenantPath, /\/api\/v1\/system\/admin\/tenants\/\$\{tenantId\}\/\$\{resource/)
  assert.match(chatHistorySettings, /usePlatformTenantControlID\(\)/)
  assert.match(chatHistorySettings, /getTenantChatHistoryConfig\(platformTenantID\.value\)/)
  assert.match(chatHistorySettings, /updateTenantChatHistoryConfig\(config, targetTenantID\)/)
  assert.match(chatHistorySettings, /getChatHistoryKBStats\(platformTenantID\.value\)/)
  assert.match(chatHistorySettings, /const modelLocked = ref\(true\)/)
  assert.match(chatHistorySettings, /const loadStats = async \(\) => \{\s*modelLocked\.value = true/)
  assert.match(chatHistorySettings, /if \(response\.data\) \{[\s\S]{0,240}modelLocked\.value = response\.data\.has_indexed_messages === true[\s\S]{0,80}else throw new Error\(t\('common\.loadFailed'\)\)/)
  assert.match(chatHistorySettings, /onUnmounted\(\(\) => \{[\s\S]{0,80}clearTimeout\(saveTimer\)/)
  assert.match(platformOperations, /openTenantSettings\('chathistory'\)/)
  assert.match(platformOperations, /:tenant-control-name="selected\?\.name"/)
})

test('nginx grants the larger skill bundle limit only to the tenant-scoped control plane', () => {
  assert.ok(nginx.includes('location ~ ^/api/v1/system/admin/tenants/[1-9][0-9]*/(?:skills/catalog|sandbox-configs/[^/]+/skills)/?$ {'))
  assert.ok(!nginx.includes('location ~ ^/api/v1/(?:skills/catalog|sandbox-configs/[^/]+/skills)/?$ {'))
})
