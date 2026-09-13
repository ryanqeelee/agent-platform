import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./ModelEditorDialog.vue', import.meta.url), 'utf8')

test('tenantless system administration only offers platform remote providers', () => {
  assert.match(
    source,
    /authStore\.isSystemAdmin\s*&& platformTenantControlID !== null\s*&& platformTenantControlID\.value === undefined/,
  )
  assert.match(
    source,
    /<button\s+v-if="!isPlatformMode"[\s\S]*?:aria-checked="formData\.source === 'local'"/,
  )
  assert.doesNotMatch(
    source,
    /<button\s+v-if="!isPlatformMode"[\s\S]*?:aria-checked="formData\.source === 'remote'"/,
  )
  assert.match(source, /options\.filter\(p => p\.value !== 'weknoracloud'\)/)
  assert.match(source, /if \(isPlatformMode\.value\) \{\s*formData\.value\.source = 'remote'\s*\} else \{\s*checkOllamaServiceStatus\(\)/)
  assert.match(source, /if \(!isPlatformMode\.value && value === 'weknoracloud'\)/)
  assert.match(
    source,
    /formData\.value\.source = normalizeModelEditorSource\(\s*formData\.value\.source,\s*activeModelType\.value,\s*isPlatformMode\.value,\s*\)/,
  )
})
