import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./Settings.vue', import.meta.url), 'utf8')

test('tenantless system admins keep the platform settings navigation', () => {
  const navItems = source.match(/const navItems = computed\(\(\) => \{([\s\S]*?)^\}\)/m)?.[1]
  assert.ok(navItems, 'expected navItems computed block')
  assert.match(
    navItems,
    /!authStore\.currentTenantRole\s*&&\s*!authStore\.effectiveCrossTenantAccess\s*&&\s*!authStore\.isSystemAdmin/,
  )
  assert.match(navItems, /return all\.filter\(\(it\) => canSeeSection\(it\.key\)/)
})
