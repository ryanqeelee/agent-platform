import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const listSource = readFileSync(new URL('./PlatformAgentList.vue', import.meta.url), 'utf8')
const editorSource = readFileSync(new URL('./AgentEditorModal.vue', import.meta.url), 'utf8')
const apiSource = readFileSync(new URL('../../api/agent/index.ts', import.meta.url), 'utf8')
const promptSelectorSource = readFileSync(new URL('../../components/PromptTemplateSelector.vue', import.meta.url), 'utf8')

test('platform agent inventory uses only the tenantless system-admin route', () => {
  assert.match(listSource, /listPlatformAgents\(\)/)
  assert.match(apiSource, /get<\{ data: CustomAgent\[\] \}>\('\/api\/v1\/system\/admin\/agents'\)/)
  assert.doesNotMatch(listSource, /useChatResourcesStore|useOrganizationStore|useResourcePins|listAgents\(/)
})

test('platform inventory only offers editing and keeps the internal installer hidden', () => {
  assert.match(listSource, /scope="platform"/)
  assert.match(listSource, /builtin-skill-installer/)
  assert.match(listSource, /builtin-wiki-fixer/)
  assert.doesNotMatch(listSource, /createAgent|deleteAgent|copyAgent|useInChat/)
  assert.doesNotMatch(listSource, /platform-agent-card__id/)
})

test('platform editor routes saves and metadata through platform endpoints', () => {
  assert.match(editorSource, /getPlatformAgentTypePresets\(\)/)
  assert.match(editorSource, /getPlatformAgentPlaceholders\(\)/)
  assert.match(editorSource, /getPlatformAgentPromptTemplates\(\)/)
  assert.match(editorSource, /await updatePlatformAgent\(formData\.value\.id/)
  assert.match(apiSource, /put<\{ data: CustomAgent \}>\(`\/api\/v1\/system\/admin\/agents\/\$\{id\}`/)
  assert.match(promptSelectorSource, /props\.scope === 'platform'[\s\S]*getPlatformAgentPromptTemplates\(\)/)
})

test('platform editor omits tenant resource discovery and bindings', () => {
  const platformBranch = editorSource.match(/if \(isPlatformMode\.value\) \{([\s\S]*?)^\s{4}\}/m)?.[1]
  assert.ok(platformBranch)
  assert.doesNotMatch(platformBranch, /ensureKnowledgeBases|fetchShared|ensureWebSearchProviders|ensureMcpServices|ensureStorageEngine/)
  assert.match(editorSource, /delete config\.knowledge_bases/)
  assert.match(editorSource, /delete config\.mcp_services/)
  assert.match(editorSource, /delete config\.sandbox_config_id/)
  assert.match(editorSource, /delete config\.web_search_provider_id/)
  assert.match(editorSource, /delete config\.chat_parser_engine_rules/)
  assert.match(editorSource, /v-model="formData\.config\.skills_selection_mode"/)
  assert.doesNotMatch(editorSource, /v-if="isPlatformMode"[\s\S]{0,800}selected_skills/)
})
