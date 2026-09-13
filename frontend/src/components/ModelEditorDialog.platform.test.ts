import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./ModelEditorDialog.vue', import.meta.url), 'utf8')

test('tenantless system administration only offers platform remote providers', () => {
  assert.match(
    source,
    /authStore\.isSystemAdmin\s*&& platformTenantControlID !== null\s*&& platformTenantControlID\.value === undefined/,
  )
  assert.match(source, /<section v-if="!isPlatformMode" class="setting-drawer__section">/)
  assert.match(source, /options\.filter\(p => p\.value !== 'weknoracloud'\)/)
  assert.match(source, /if \(isPlatformMode\.value\) \{\s*formData\.value\.source = 'remote'\s*\} else \{\s*checkOllamaServiceStatus\(\)/)
  assert.match(source, /if \(!isPlatformMode\.value && value === 'weknoracloud'\)/)
})
