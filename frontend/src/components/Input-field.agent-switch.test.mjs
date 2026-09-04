import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const inputField = readFileSync(new URL('./Input-field.vue', import.meta.url), 'utf8')
const settingsStore = readFileSync(new URL('../stores/settings.ts', import.meta.url), 'utf8')

test('selecting a search-capable agent enables web search by default', () => {
  const selectAgentStart = settingsStore.indexOf('selectAgent(agentId: string')
  const getSelectedAgentStart = settingsStore.indexOf('getSelectedAgentId()', selectAgentStart)
  const selectAgentAction = settingsStore.slice(selectAgentStart, getSelectedAgentStart)

  assert.notEqual(selectAgentStart, -1)
  assert.notEqual(getSelectedAgentStart, -1)
  assert.match(selectAgentAction, /this\.settings\.webSearchEnabled = true/)

  const handleSelectAgentStart = inputField.indexOf('const handleSelectAgent = async')
  const handleSelectAgentEnd = inputField.indexOf('const clearvalue', handleSelectAgentStart)
  const handleSelectAgent = inputField.slice(handleSelectAgentStart, handleSelectAgentEnd)

  assert.notEqual(handleSelectAgentStart, -1)
  assert.notEqual(handleSelectAgentEnd, -1)
  assert.match(handleSelectAgent, /settingsStore\.selectAgent\(agent\.id, sourceTenantId\)/)
  assert.doesNotMatch(handleSelectAgent, /agentWebSearch/)
  assert.doesNotMatch(handleSelectAgent, /settingsStore\.toggleWebSearch/)
})

test('search capability stays visible while provider readiness controls its disabled state', () => {
  const showWebSearchStart = inputField.indexOf('const showWebSearchButton = computed')
  const showWebSearchEnd = inputField.indexOf('const showImageUploadButton', showWebSearchStart)
  const showWebSearchButton = inputField.slice(showWebSearchStart, showWebSearchEnd)

  assert.notEqual(showWebSearchStart, -1)
  assert.notEqual(showWebSearchEnd, -1)
  assert.match(showWebSearchButton, /isWebSearchReadinessKnown/)
  assert.match(showWebSearchButton, /isWebSearchEnabledByAgent\.value === true/)
  assert.doesNotMatch(showWebSearchButton, /isAgentWebSearchReady/)
})

test('images remain attachable when the selected agent has no direct vision channel', () => {
  assert.match(inputField, /const showImageUploadButton = computed\(\(\) => true\)/)
  assert.match(inputField, /else void attachmentUploadRef\.value\?\.addFiles\(files\)/)
  assert.match(inputField, /void attachmentUploadRef\.value\?\.addFiles\(imageFiles\)/)
})

test('the agent picker reports the available image attachment path as supported', () => {
  const selector = readFileSync(new URL('./AgentSelector.vue', import.meta.url), 'utf8')
  assert.match(selector, /imageUploadCapability[\s\S]*capabilitySupported/)
  assert.doesNotMatch(selector, /getImageUploadCapabilityState/)
})

test('employee viewers do not call management-only MCP discovery', () => {
  const resourcesStart = inputField.indexOf('const resources = [')
  const managementGate = inputField.indexOf('if (canManageAgents.value) resources.push', resourcesStart)

  assert.notEqual(resourcesStart, -1)
  assert.notEqual(managementGate, -1)
  assert.doesNotMatch(inputField.slice(resourcesStart, managementGate), /loadMCPServices\(\)/)
  assert.match(inputField, /if \(canManageAgents\.value\) resources\.push\(loadMCPServices\(\)\)/)
})
