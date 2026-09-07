import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const platform = readFileSync(fileURLToPath(new URL('../platform/index.vue', import.meta.url)), 'utf8')
const workspace = readFileSync(fileURLToPath(new URL('./OperatingWorkspace.vue', import.meta.url)), 'utf8')
const client = readFileSync(fileURLToPath(new URL('./operatingClient.ts', import.meta.url)), 'utf8')
const menu = readFileSync(fileURLToPath(new URL('../../components/menu.vue', import.meta.url)), 'utf8')
const router = readFileSync(fileURLToPath(new URL('../../router/index.ts', import.meta.url)), 'utf8')

test('the platform shell keeps one native operating host after the first authorized visit', () => {
  assert.match(platform, /<OperatingWorkspace[\s\S]*?v-if="operatingWorkspaceMounted"[\s\S]*?v-show="isOperatingRoute"/)
  assert.match(platform, /if \(isOperating\) operatingWorkspaceMounted\.value = true/)
  assert.match(platform, /<Menu id="mobile-workspace-menu" :operating-controller="operatingController"[^>]*><\/Menu>[\s\S]*?<OperatingWorkspace/)
  assert.doesNotMatch(workspace, /<iframe|postMessage|contentWindow|buildEmbeddedOperatingPath/)
  assert.match(workspace, /<InputField :operating="operatingComposer"/)
  assert.match(workspace, /<AgentStreamDisplay[\s\S]*?process-only[\s\S]*?:operating-status="message\.status"/)
  assert.match(workspace, /@operating-result="inspectQuery"/)
  assert.match(workspace, /v-if="message\.report"[\s\S]*?selectReport\(message\.report\.artifactId\)/)
  assert.match(workspace, /v-if="snapshot\.sessionLoadError"[\s\S]*?重试加载[\s\S]*?controller\.retrySession\(\)/)
  assert.match(workspace, /v-else-if="snapshot\.messages\.length === 0" class="operating-welcome"/)
  assert.match(workspace, /<ChatReadingPanel[\s\S]*?variant="reading"[\s\S]*?ref="reportTarget"/)
  assert.match(workspace, /<usermsg v-if="message\.role === 'user'"/)
  assert.match(platform, /if \(isChatDropRoute\(\) \|\| isOperatingRoute\.value\)/)
})

test('the runtime loads only through the fixed same-origin ESM entry and owns no visible chat', () => {
  assert.match(client, /OPERATING_CLIENT_MODULE_PATH = '\/app\/operating-client\.js'/)
  assert.match(client, /import\(\/\* @vite-ignore \*\/ OPERATING_CLIENT_MODULE_PATH\)/)
  assert.doesNotMatch(client, /window\[|globalThis\[|eval\(|remoteUrl|baseUrl/)
  assert.match(workspace, /ref="runtimeRoot" class="operating-runtime-root" aria-hidden="true"/)
  assert.match(workspace, /\.operating-runtime-root \{ display: none; \}/)
  assert.match(workspace, /controller\.mountReport\(reportTarget\.value\)/)
  assert.match(workspace, /controller\.mountControls\(controlsTarget\.value\)/)
  assert.match(workspace, /controller\.mountBrief\(briefTarget\.value\)/)
})

test('route changes activate the existing controller while runtime navigation alone writes history', () => {
  assert.match(workspace, /controller\.navigate\(location\)/)
  assert.match(workspace, /controller\.setActive\(true\)/)
  assert.match(workspace, /controller\?\.setActive\(false\)/)
  assert.match(workspace, /runtimeNavigationTarget\(active\.value, route\.path, route\.query, location\)/)
  assert.match(workspace, /void router\[mode\]\(target\)/)
  assert.match(workspace, /controller\?\.dispose\(\)/)
})

test('existing route authorization and employee handoff storage remain the entry boundary', () => {
  assert.doesNotMatch(router, /window\.location\.assign\(handoffPrompt/)
  assert.match(router, /sessionStorage\.setItem\(\s*OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY/)
  assert.match(router, /localStorage\.setItem\('retail_ai_app_auth_token', response\.access_token\)/)
  assert.match(router, /return true/)
})

test('native sidebar reads operating history from the mounted Center controller', () => {
  assert.match(platform, /const operatingController = shallowRef<OperatingController \| null>\(null\)/)
  assert.match(platform, /@controller-change="handleOperatingControllerChange"/)
  assert.match(workspace, /emit\('controller-change', controller\)/)
  assert.match(workspace, /emit\('controller-change', null\)/)
  assert.match(menu, /data-session-source="operating-controller"/)
  assert.match(menu, /<SessionSidebarRow[\s\S]*?:menu-options="operatingSessionMenuOptions"/)
  assert.match(menu, /nextController\.getSnapshot\(\)/)
  assert.match(menu, /nextController\.subscribe\(publishOperatingSnapshot\)/)
  assert.match(menu, /groupSessionsByDate\([\s\S]*?classifyDateBucket\(session\.updated_at\)/)
  assert.match(menu, /operatingRouteLocationFromRuntime\(operatingSessionLocation\(current, id\)\)/)
  assert.doesNotMatch(menu, /operatingController\?\.openSession/)
  assert.match(menu, /props\.operatingController\?\.renameSession\(id, title\)/)
  assert.match(menu, /props\.operatingController\?\.deleteSession\(id\)/)
  assert.doesNotMatch(workspace, /class="operating-history"/)
  assert.doesNotMatch(menu, /operatingSessionMenuOptions[\s\S]{0,200}(?:pin|clearMessages)/)
})

test('employee conversation history remains separate from operating history', () => {
  assert.match(menu, /class="submenu" v-else-if="!uiStore\.sidebarCollapsed"/)
  assert.match(menu, /route\.path === '\/platform\/operating-brief' \|\| route\.path === '\/platform\/operating-analysis'/)
})
