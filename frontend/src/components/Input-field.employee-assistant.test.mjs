import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { employeeWebSearchEnabled } from '../api/agent/constants.ts'

const inputField = readFileSync(new URL('./Input-field.vue', import.meta.url), 'utf8')
const chat = readFileSync(new URL('../views/chat/index.vue', import.meta.url), 'utf8')
const createChat = readFileSync(new URL('../views/creatChat/creatChat.vue', import.meta.url), 'utf8')
const chatResources = readFileSync(new URL('../stores/chatResources.ts', import.meta.url), 'utf8')
const commandSearch = readFileSync(new URL('./GlobalCommandPalette/useSearch.ts', import.meta.url), 'utf8')

test('employee input has no agent selector or switch handler', () => {
  assert.doesNotMatch(inputField, /AgentSelector/)
  assert.doesNotMatch(inputField, /handleSelectAgent/)
  assert.doesNotMatch(inputField, /selectedAgentSourceTenantId/)
})

test('web search readiness comes from unified agent server projection', () => {
  assert.match(inputField, /employeeAssistantProjection[\s\S]*BUILTIN_EMPLOYEE_ASSISTANT_ID/)
  assert.match(inputField, /typeof employeeAssistantProjection\.value\?\.web_search_ready === 'boolean'/)
  assert.match(inputField, /employeeWebSearchEnabled\(true, employeeAssistantProjection\.value\)/)
  assert.doesNotMatch(inputField, /ensureWebSearchProviders/)
  assert.doesNotMatch(inputField, /getWebSearchProviders/)
  assert.match(inputField, /input\.webSearch\.enterpriseUnavailable/)
})

test('ordinary employee prefetch does not load provider catalog or agent choices', () => {
  const prefetchStart = chatResources.indexOf('async function prefetchChatInput')
  const prefetchEnd = chatResources.indexOf('async function ensureAgentKnowledgeBases', prefetchStart)
  const prefetch = chatResources.slice(prefetchStart, prefetchEnd)

  assert.notEqual(prefetchStart, -1)
  assert.notEqual(prefetchEnd, -1)
  assert.doesNotMatch(prefetch, /ensureWebSearchProviders/)
  assert.doesNotMatch(commandSearch, /ensureAgents|agentMatches|CmdkAgent/)
})

test('employee mixed uploads and request always use unified assistant', () => {
  const employeeIdExpression = /const selectedAgentId = props\.embeddedMode \? props\.agentId : BUILTIN_EMPLOYEE_ASSISTANT_ID;/
  assert.match(chat, employeeIdExpression)
  assert.match(chat, /uploadTemporaryAttachment\([\s\S]*selectedAgentId, undefined, 'auto'/)
  assert.match(chat, /agent_enabled: agentEnabled,[\s\S]*agent_id: selectedAgentId/)
  assert.doesNotMatch(chat, /agent_source_tenant_id:/)
  assert.doesNotMatch(chat, /route\.query\.agent_id/)
  assert.doesNotMatch(chat, /route\.query\.agent_source_tenant_id/)
  assert.match(chat, /const endpoint = props\.embeddedMode && !agentEnabled \? '\/api\/v1\/knowledge-chat' : '\/api\/v1\/agent-chat';/)
})

test('new employee sessions do not snapshot local agent execution config', () => {
  assert.match(createChat, /createSessions\(\{\}\)/)
  assert.doesNotMatch(createChat, /sessionData\.agent_config/)
})

test('employee send masks optional web permission by tenant capability and readiness', () => {
  for (const entitlement of [true, false, undefined]) {
    for (const readiness of [true, false, undefined]) {
      for (const permission of [true, false]) {
        assert.equal(employeeWebSearchEnabled(permission, {
          config: { web_search_enabled: entitlement }, web_search_ready: readiness,
        }), permission && entitlement === true && readiness === true)
      }
    }
  }
  assert.equal(employeeWebSearchEnabled(true, undefined), false)
  assert.match(chat, /employeeWebSearchEnabled\([\s\S]*useSettingsStoreInstance\.isWebSearchEnabled,[\s\S]*useChatResourcesStore\(\)\.agents\.find/)
})
