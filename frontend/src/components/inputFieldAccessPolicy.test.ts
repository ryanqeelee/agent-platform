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
  const commandPalette = await readFile(new URL('./GlobalCommandPalette.vue', import.meta.url), 'utf8')

  assert.match(commandPalette, /command\.id === 'open-kb-list'[\s\S]*EMPLOYEE_SURFACE_MIN_ROLE\.knowledgeBases/)
  assert.match(commandPalette, /command\.id === 'open-agents'[\s\S]*EMPLOYEE_SURFACE_MIN_ROLE\.agents/)
  assert.match(commandPalette, /v-if="canConfigureRetrieval"/)
  assert.match(commandPalette, /<t-drawer v-if="canConfigureRetrieval"/)
  assert.match(commandPalette, /<t-button v-if="canConfigureRetrieval" variant="outline" size="small"/)
})
