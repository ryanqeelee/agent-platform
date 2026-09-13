import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const systemAPI = readFileSync(new URL('./system/index.ts', import.meta.url), 'utf8')
const deploymentCapabilitiesStore = readFileSync(new URL('../stores/deploymentCapabilities.ts', import.meta.url), 'utf8')
const skillAPI = readFileSync(new URL('./skill/index.ts', import.meta.url), 'utf8')
const chatHistoryAPI = readFileSync(new URL('./chat-history.ts', import.meta.url), 'utf8')
const chatHistoryPath = readFileSync(new URL('./chat-history-path.ts', import.meta.url), 'utf8')
const settings = readFileSync(new URL('../views/settings/Settings.vue', import.meta.url), 'utf8')
const chatHistorySettings = readFileSync(new URL('../views/settings/ChatHistorySettings.vue', import.meta.url), 'utf8')
const platformOperations = readFileSync(new URL('../views/operations/PlatformOperations.vue', import.meta.url), 'utf8')
const nginx = readFileSync(new URL('../../nginx.conf', import.meta.url), 'utf8')

test('platform sandbox and skill APIs are tenantless', () => {
  assert.match(systemAPI, /\/api\/v1\/system\/admin\/sandbox-configs/)
  assert.match(skillAPI, /\/api\/v1\/system\/admin\/skills/)
  assert.doesNotMatch(systemAPI, /system\/admin\/tenants\/[^'`\n]*sandbox-configs/)
  assert.doesNotMatch(skillAPI, /system\/admin\/tenants\/[^'`\n]*skills/)
  assert.doesNotMatch(systemAPI, /['`]\/api\/v1\/sandbox-configs/)
  assert.doesNotMatch(skillAPI, /['`]\/api\/v1\/skills/)
})

test('deployment capabilities use the tenantless system-admin route for platform login', () => {
  assert.match(systemAPI, /getDeploymentCapabilities\(systemAdmin = false\)/)
  assert.match(systemAPI, /systemAdmin \? '\/api\/v1\/system\/admin\/capabilities' : '\/api\/v1\/system\/capabilities'/)
  assert.match(deploymentCapabilitiesStore, /getDeploymentCapabilities\(useAuthStore\(\)\.isSystemAdmin\)/)
})

test('system info selects the platform path only from an explicit argument', () => {
  const systemInfo = readFileSync(new URL('../views/settings/SystemInfo.vue', import.meta.url), 'utf8')

  assert.match(systemAPI, /getSystemInfo\(systemAdmin = false\)/)
  assert.match(systemAPI, /systemAdmin \? '\/api\/v1\/system\/admin\/info' : '\/api\/v1\/system\/info'/)
  assert.match(systemInfo, /getSystemInfo\(authStore\.isSystemAdmin\)/)
})

test('tenant parser catalog is separate from tenantless system-admin management', () => {
  assert.match(systemAPI, /getParserEngines\(\)[\s\S]{0,120}get\('\/api\/v1\/system\/parser-engines'\)/)
  assert.match(systemAPI, /getAdminParserEngines\(\)[\s\S]{0,120}get\('\/api\/v1\/system\/admin\/parser-engines'\)/)
  assert.match(systemAPI, /post\('\/api\/v1\/system\/admin\/parser-engines\/check', config\)/)
  assert.match(systemAPI, /get\('\/api\/v1\/system\/admin\/parser-engine-config'\)/)
  assert.match(systemAPI, /put\('\/api\/v1\/system\/admin\/parser-engine-config', config\)/)
  assert.doesNotMatch(systemAPI, /parser-engine-config[^\n]*tenantId/)
  assert.doesNotMatch(systemAPI, /docreader\/reconnect/)
})

test('platform settings do not depend on a selected enterprise', () => {
  assert.doesNotMatch(settings, /tenantControlId|tenantControlName|platformTenantControlIDKey/)
  assert.match(settings, /<ChatHistorySettings\s*\/>/)
  assert.match(settings, /<MemoryWorkspaceSettings :key="currentSection"/)
  assert.match(settings, /settingsSurface === 'enterprise'/)
})

test('message indexing uses the tenantless platform control-plane path', () => {
  assert.match(chatHistoryAPI, /platformChatHistoryPath\('chat-history-config'\)/)
  assert.match(chatHistoryPath, /\/api\/v1\/system\/admin\/\$\{resource\}/)
  assert.doesNotMatch(chatHistorySettings, /usePlatformTenantControlID\(\)/)
  assert.match(chatHistorySettings, /getPlatformChatHistoryConfig\(\)/)
  assert.match(chatHistorySettings, /updatePlatformChatHistoryConfig\(config\)/)
  assert.match(chatHistorySettings, /getPlatformChatHistoryStats\(\)/)
  assert.match(chatHistorySettings, /const modelLocked = ref\(true\)/)
  assert.match(chatHistorySettings, /const loadStats = async \(\) => \{\s*modelLocked\.value = true/)
  assert.match(chatHistorySettings, /modelLocked\.value = response\.data\.tenant_knowledge_base_count > 0/)
  assert.match(chatHistorySettings, /onUnmounted\(\(\) => \{[\s\S]{0,80}clearTimeout\(saveTimer\)/)
  assert.match(platformOperations, /uiStore\.openSettings\('chathistory'\)/)
  assert.doesNotMatch(platformOperations, /tenant-control|openTenantSettings|tenant-settings-hint/)
})

test('nginx grants the larger skill bundle limit only to the platform control plane', () => {
  assert.ok(nginx.includes('location ~ ^/api/v1/system/admin/(?:skills|sandbox-configs/[^/]+/skills)/?$ {'))
  assert.ok(!nginx.includes('location ~ ^/api/v1/(?:skills|sandbox-configs/[^/]+/skills)/?$ {'))
})
