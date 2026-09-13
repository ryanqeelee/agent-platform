import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const skillAPI = readFileSync(new URL('./skill/index.ts', import.meta.url), 'utf8')
const skillSettings = readFileSync(new URL('../views/settings/SkillSettings.vue', import.meta.url), 'utf8')
const sandboxSkillsPanel = readFileSync(new URL('../components/SandboxSkillsPanel.vue', import.meta.url), 'utf8')

test('skill installer agent uses the tenantless platform agent path', () => {
  assert.match(skillAPI, /getSkillInstallerAgent\(\)/)
  assert.match(skillAPI, /updateSkillInstallerAgent\(data: UpdateAgentRequest\)/)
  assert.match(skillAPI, /\/api\/v1\/system\/admin\/agents\/builtin-skill-installer/)
  assert.doesNotMatch(skillAPI, /\/api\/v1\/agents/)
  assert.doesNotMatch(skillAPI, /tenantId/)
})

test('platform skill panels do not call the enterprise agent API', () => {
  for (const source of [skillSettings, sandboxSkillsPanel]) {
    assert.match(source, /getSkillInstallerAgent\(\)/)
    assert.match(source, /updateSkillInstallerAgent\(\{/)
    assert.doesNotMatch(source, /\bgetAgentById\b/)
    assert.doesNotMatch(source, /\bupdateAgent\b/)
    assert.doesNotMatch(source, /platformTenantID/)
  }
})
