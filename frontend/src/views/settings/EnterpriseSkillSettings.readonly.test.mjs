import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const page = readFileSync(new URL('./EnterpriseSkillSettings.vue', import.meta.url), 'utf8')
const skillAPI = readFileSync(new URL('../../api/skill/index.ts', import.meta.url), 'utf8')

test('enterprise skill settings use only the read-only employee projection', () => {
  assert.match(page, /listEmployeeSkills\(\)/)
  assert.match(skillAPI, /listEmployeeSkills\(\)[\s\S]{0,160}\/api\/v1\/employee-assistant\/skills/)
  assert.match(page, /skill\.name/)
  assert.match(page, /skill\.description/)

  for (const source of [page, skillAPI]) {
    assert.doesNotMatch(source, /employee-assistant\/skills\/manage/)
  }
  assert.doesNotMatch(page, /\bpatch\s*\(/)
  assert.doesNotMatch(page, /<t-switch/)
  assert.doesNotMatch(page, /skill\.(?:enabled|status|id)/)
})
