import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('workspace users cannot select a concrete chat model', async () => {
  const source = await readFile(new URL('./Input-field.vue', import.meta.url), 'utf8')

  assert.match(source, /const canSelectChatModel = computed\(\(\) => authStore\.isSystemAdmin\);/)
  assert.match(source, /<t-tooltip v-if="canSelectChatModel"/)
  assert.match(source, /<div v-if="canSelectChatModel && showModelSelector" class="model-selector-overlay"/)
  assert.match(source, /const toggleModelSelector = \(\) => \{\s*if \(!canSelectChatModel\.value\) return;/)
})

test('viewer management shortcuts reuse the employee surface role policy', async () => {
  const [inputField, agentSelector, commandPalette] = await Promise.all([
    readFile(new URL('./Input-field.vue', import.meta.url), 'utf8'),
    readFile(new URL('./AgentSelector.vue', import.meta.url), 'utf8'),
    readFile(new URL('./GlobalCommandPalette.vue', import.meta.url), 'utf8'),
  ])

  assert.match(agentSelector, /<router-link v-if="canManageAgents" to="\/platform\/agents"/)
  assert.match(agentSelector, /if \(!canManageAgents\.value\) return false;/)
  assert.match(commandPalette, /command\.id === 'open-kb-list'[\s\S]*EMPLOYEE_SURFACE_MIN_ROLE\.knowledgeBases/)
  assert.match(commandPalette, /command\.id === 'open-agents'[\s\S]*EMPLOYEE_SURFACE_MIN_ROLE\.agents/)
  assert.match(commandPalette, /v-if="canConfigureRetrieval"/)
  assert.match(commandPalette, /<t-drawer v-if="canConfigureRetrieval"/)
  assert.match(commandPalette, /<t-button v-if="canConfigureRetrieval" variant="outline" size="small"/)
  assert.match(inputField, /const canManageAgents = computed[\s\S]*EMPLOYEE_SURFACE_MIN_ROLE\.agents/)
  assert.match(inputField, /if \(!canManageAgents\.value\) return;/)
  assert.match(inputField, /isRemoteShared \|\| !canManageAgents\.value/)
  assert.match(inputField, /<a v-if="canManageAgents" href="#" @click\.prevent="handleGoToAgentSettings\('knowledge'\)"/)
})
