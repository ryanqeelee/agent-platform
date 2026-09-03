import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const routerSource = readFileSync(fileURLToPath(new URL('./index.ts', import.meta.url)), 'utf8')
const platformSource = readFileSync(fileURLToPath(new URL('../views/platform/index.vue', import.meta.url)), 'utf8')

test('the settings route addresses the single overlay owned by the platform shell', () => {
  assert.match(platformSource, /<Settings\s*\/>/)
  assert.match(
    routerSource,
    /path: "settings",[\s\S]*?name: "settings",[\s\S]*?component: \{ render: \(\) => null \}/,
  )
  assert.doesNotMatch(routerSource, /path: "settings",[\s\S]{0,180}views\/settings\/Settings\.vue/)
})
