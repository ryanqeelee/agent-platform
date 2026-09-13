import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8')

test('storage and vector administration use tenantless platform routes', () => {
  const storage = read('./storage-backend.ts')
  const vector = read('./vector-store.ts')

  assert.match(storage, /const basePath = '\/api\/v1\/system\/admin\/storage-backends'/)
  assert.match(vector, /const basePath = '\/api\/v1\/system\/admin\/vector-stores'/)
  assert.doesNotMatch(storage, /platformTenantPath|tenantId/)
  assert.doesNotMatch(vector, /platformTenantPath|tenantId/)
})

test('knowledge-base capability projections expose explicit platform defaults', () => {
  const resources = read('../stores/editorResources.ts')
  const editor = read('../views/knowledge/KnowledgeBaseEditorModal.vue')
  const storageSettings = read('../views/knowledge/settings/KBStorageSettings.vue')
  const vectorSettings = read('../views/knowledge/settings/KBVectorStoreSettings.vue')

  assert.match(resources, /defaultStorageBackendID\.value = response\?\.default_storage_backend_id \?\? ''/)
  assert.match(resources, /defaultVectorStoreID\.value = response\?\.default_vector_store_id \?\? ''/)
  assert.match(editor, /formData\.value\.storageBackendId = editorResources\.defaultStorageBackendID/)
  assert.match(editor, /formData\.value\.vectorStoreId = editorResources\.defaultVectorStoreID/)
  assert.match(storageSettings, /if \(!localID\.value\) localID\.value = defaultID\.value/)
  assert.doesNotMatch(storageSettings, /backends\.value\[0\]/)
  assert.match(vectorSettings, /localVectorStoreId\.value = defaultVectorStoreId\.value/)
  assert.doesNotMatch(editor, /storage_provider_config|storage_config\s*=/)
})
