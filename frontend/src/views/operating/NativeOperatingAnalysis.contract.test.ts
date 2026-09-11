import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const read = (relative: string) => readFileSync(fileURLToPath(new URL(relative, import.meta.url)), 'utf8')
const home = read('./OperatingAnalysisHome.vue')
const router = read('../../router/index.ts')
const chat = read('../chat/index.vue')
const input = read('../../components/Input-field.vue')
const menu = read('../../components/menu.vue')
const platform = read('../platform/index.vue')
const brief = read('./OperatingBriefWorkspace.vue')

test('new operating work enters one fixed native agent without URL agent selection', () => {
  assert.match(router, /path: "operating-analysis"[\s\S]*?OperatingAnalysisHome\.vue/)
  assert.match(router, /path: 'operating-analysis\/chat\/:chatid'[\s\S]*?agentId: BUILTIN_OPERATING_ANALYST_ID/)
  assert.doesNotMatch(router, /query\.(?:agent|agent_id)/)
  assert.match(home, /:agent-id="BUILTIN_OPERATING_ANALYST_ID"/)
  assert.match(home, /createSessions\(\{\}\)/)
  assert.match(home, /router\.push\(`\/platform\/operating-analysis\/chat\/\$\{sessionId\}`\)/)
  assert.match(chat, /agent_id: selectedAgentId/)
})

test('operating keeps the existing home copy and native employee input capabilities', () => {
  assert.match(home, /今天想先看哪项经营变化？/)
  assert.match(home, /看销售、查毛利、找变化。可以直接提问，或上传经营数据文件。/)
  assert.match(input, /websearch-btn/)
  assert.match(input, /image-upload-btn/)
  assert.match(input, /attachment-upload-btn/)
  assert.match(input, /data-guide="chat-kb-mention"/)
  assert.match(input, /v-if="!agentId"[\s\S]*?assistant-mode-select/)
  assert.match(input, /v-if="canSelectChatModel && !agentId"/)
  assert.doesNotMatch(home, /<select|assistant-mode-select|深入处理/)
})

test('the fixed operating agent sends the explicit per-turn web switch', () => {
  assert.match(input, /emit\('send-msg'[\s\S]*?isWebSearchEnabled\.value && isWebSearchConfigured\.value\)/)
  assert.match(home, /changeFirstQuery\(value, mentionedItems, modelId, imageFiles, attachmentFiles, webSearchEnabled\)/)
  assert.match(chat, /firstWebSearchEnabled\.value/)
  assert.match(chat, /isNativeOperatingTrial\.value[\s\S]*?\? fixedAgentWebSearchEnabled/)
  assert.match(chat, /web_search_enabled: webSearchEnabled/)
})

test('platform-native agents reuse the server-owned employee skill catalog', () => {
  assert.match(input, /platformSkillSemanticAgentIds = new Set\(\[[\s\S]*?BUILTIN_EMPLOYEE_ASSISTANT_ID[\s\S]*?'builtin-data-analysis-base'[\s\S]*?BUILTIN_OPERATING_ANALYST_ID/)
  assert.match(input, /platformSkillSemanticAgentIds\.has\(selectedAgentId\.value\)[\s\S]*?\? BUILTIN_EMPLOYEE_ASSISTANT_ID/)
  assert.match(input, /if \(skillsMode !== 'none'\)/)
})

test('the brief is native Vue and the retired Center browser only redirects home', () => {
  assert.match(router, /path: 'operating-analysis\/history\/:sessionId\?'[\s\S]*?redirect: '\/platform\/operating-analysis'/)
  assert.match(platform, /OperatingBriefWorkspace/)
  assert.doesNotMatch(platform, /OperatingWorkspace|operatingController/)
  assert.doesNotMatch(home, /迁移前的历史分析|operating-analysis\/history/)
  assert.doesNotMatch(router, /OperatingAnalysisHistory|authorizeOperatingDataRead/)
  assert.doesNotMatch(brief, /mountOperatingBrief|\/app\/operating-brief\.js|retail_ai_app_auth_token|document\.cookie/)
  assert.doesNotMatch(menu, /operatingController|operating-client/)
})

test('native chat and brief routes remain addressable after browser retirement', () => {
  assert.match(router, /path: 'operating-analysis\/chat\/:chatid'[\s\S]*?agentId: BUILTIN_OPERATING_ANALYST_ID/)
  assert.match(router, /path: 'operating-brief'[\s\S]*?name: 'operatingBrief'/)
  assert.match(router, /retiredOperatingAnalysisQueryRedirect\(to\.query\)/)
})
