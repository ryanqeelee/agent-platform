import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8')

test('vector-store badge does not synthesize the removed env source', () => {
  const badge = read('./VectorStoreBadge.vue')
  const api = read('../api/knowledge-base/index.ts')

  assert.match(api, /VectorStoreSource = 'user' \| 'shared' \| 'unavailable'/)
  assert.doesNotMatch(api, /VectorStoreSource = [^;]*'env'/)
  assert.match(badge, /props\.source \|\| 'unavailable'/)
  assert.doesNotMatch(badge, /effectiveSource\.value === 'env'|case 'env'|vs-badge-env/)
})
