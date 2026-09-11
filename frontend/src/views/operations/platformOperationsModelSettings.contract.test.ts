import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const source = readFileSync(fileURLToPath(new URL('./PlatformOperations.vue', import.meta.url)), 'utf8')

test('platform operations opens the existing model settings overlay', () => {
  assert.match(source, /import Settings from '@\/views\/settings\/Settings\.vue'/)
  assert.match(source, /const uiStore = useUIStore\(\)/)
  assert.match(source, /@click="uiStore\.openSettings\('models'\)"[^>]*>模型配置<\/t-button>/)
  assert.match(source, /<Settings\s*\/>/)
})

test('model settings protects YAML lifecycle while allowing manual global cleanup', () => {
  const modelSettings = readFileSync(
    fileURLToPath(new URL('../settings/ModelSettings.vue', import.meta.url)),
    'utf8',
  )
  assert.match(modelSettings, /managedBy: model\.managed_by \|\| ''/)
  assert.match(modelSettings, /authStore\.isSystemAdmin && model\.managedBy !== 'yaml'/)
  assert.match(modelSettings, /model\?\.managed_by === 'yaml'/)
  assert.match(modelSettings, /<PlatformRuntimeContext v-if="authStore\.hasValidTenant" \/>/)
})
