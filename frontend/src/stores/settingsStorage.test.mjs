import assert from 'node:assert/strict'
import test from 'node:test'
import {
  SETTINGS_STORAGE_KEY,
  cloneSettings,
  isStoredSettingsRecord,
  loadAndReconcileSettings,
} from './settingsStorage.ts'
import { BUILTIN_EMPLOYEE_ASSISTANT_ID } from '../api/agent/constants.ts'

function makeDefaults() {
  return {
    isAgentEnabled: true,
    webSearchEnabled: true,
    selectedAgentId: BUILTIN_EMPLOYEE_ASSISTANT_ID,
    selectedAgentSourceTenantId: null,
    selectedKnowledgeBases: [],
    selectedFiles: [],
    selectedTags: [],
    selectedMCPServices: [],
    selectedSkills: [],
    selectedFileKbMap: {},
    nested: { items: ['a'] },
  }
}

function installMockLocalStorage() {
  const store = {}
  Object.defineProperty(globalThis, 'localStorage', {
    value: {
      getItem: key => (key in store ? store[key] : null),
      setItem: (key, value) => { store[key] = value },
      removeItem: key => { delete store[key] },
    },
    configurable: true,
    writable: true,
  })
  return store
}

test('isStoredSettingsRecord rejects non-object JSON values', () => {
  assert.equal(isStoredSettingsRecord(null), false)
  assert.equal(isStoredSettingsRecord([]), false)
  assert.equal(isStoredSettingsRecord('x'), false)
  assert.equal(isStoredSettingsRecord({}), true)
})

test('cloneSettings deep-clones nested structures', () => {
  const defaults = makeDefaults()
  const cloned = cloneSettings(defaults)
  cloned.nested.items.push('b')
  assert.deepEqual(defaults.nested.items, ['a'])
})

test('legacy saved agent selection is clamped without losing attachments or KB scope', () => {
  const store = installMockLocalStorage()
  store[SETTINGS_STORAGE_KEY] = JSON.stringify({
    isAgentEnabled: false,
    selectedAgentId: 'shared-agent-from-another-tenant',
    selectedAgentSourceTenantId: 'source-tenant',
    selectedKnowledgeBases: ['kb-1'],
    selectedFiles: ['file-1'],
    selectedFileKbMap: { 'file-1': 'kb-1' },
  })

  const loaded = loadAndReconcileSettings(makeDefaults())

  assert.equal(loaded.isAgentEnabled, true)
  assert.equal(loaded.selectedAgentId, BUILTIN_EMPLOYEE_ASSISTANT_ID)
  assert.equal(loaded.selectedAgentSourceTenantId, null)
  assert.equal(loaded.webSearchEnabled, true)
  assert.deepEqual(loaded.selectedKnowledgeBases, ['kb-1'])
  assert.deepEqual(loaded.selectedFiles, ['file-1'])
  assert.deepEqual(loaded.selectedFileKbMap, { 'file-1': 'kb-1' })
  assert.equal(JSON.parse(store[SETTINGS_STORAGE_KEY]).selectedAgentId, BUILTIN_EMPLOYEE_ASSISTANT_ID)
})

test('missing source tenant is normalized to explicit null', () => {
  const store = installMockLocalStorage()
  store[SETTINGS_STORAGE_KEY] = JSON.stringify({
    isAgentEnabled: true,
    selectedAgentId: BUILTIN_EMPLOYEE_ASSISTANT_ID,
  })

  const loaded = loadAndReconcileSettings(makeDefaults())

  assert.equal(loaded.selectedAgentSourceTenantId, null)
  assert.equal(JSON.parse(store[SETTINGS_STORAGE_KEY]).selectedAgentSourceTenantId, null)
})

test('missing storage returns independent canonical defaults', () => {
  const store = installMockLocalStorage()
  const defaults = makeDefaults()

  const loaded = loadAndReconcileSettings(defaults)
  loaded.selectedTags.push('tag-1')

  assert.deepEqual(defaults.selectedTags, [])
  assert.equal(loaded.selectedAgentId, BUILTIN_EMPLOYEE_ASSISTANT_ID)
  assert.equal(store[SETTINGS_STORAGE_KEY], undefined)
})

test('corrupted storage resets to canonical defaults', () => {
  const store = installMockLocalStorage()
  store[SETTINGS_STORAGE_KEY] = '{broken'

  const loaded = loadAndReconcileSettings(makeDefaults())

  assert.equal(store[SETTINGS_STORAGE_KEY], undefined)
  assert.equal(loaded.selectedAgentId, BUILTIN_EMPLOYEE_ASSISTANT_ID)
  assert.equal(loaded.isAgentEnabled, true)
})
