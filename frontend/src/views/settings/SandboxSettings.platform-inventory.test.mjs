import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./SandboxSettings.vue', import.meta.url), 'utf8')

test('platform sandbox inventory shows IDs without enterprise session API or navigation', () => {
  assert.match(source, /<span class="inventory-row__title" :title="id">\{\{ id \}\}<\/span>/)
  assert.doesNotMatch(source, /@\/api\/chat/)
  assert.doesNotMatch(source, /getSession/)
  assert.doesNotMatch(source, /useRouter|router\.push/)
  assert.doesNotMatch(source, /\/platform\/chat/)
  assert.doesNotMatch(source, /@click="openSession/)
})
