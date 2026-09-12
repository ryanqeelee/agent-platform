import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const skillAPI = readFileSync(new URL('./skill/index.ts', import.meta.url), 'utf8')
const skillSettings = readFileSync(new URL('../views/settings/SkillSettings.vue', import.meta.url), 'utf8')
const sandboxSkillsPanel = readFileSync(new URL('../components/SandboxSkillsPanel.vue', import.meta.url), 'utf8')

test('skill installer agent uses only the selected enterprise control path', () => {
  assert.match(skillAPI, /getSkillInstallerAgent\(tenantId: number\)/)
  assert.match(skillAPI, /updateSkillInstallerAgent\(tenantId: number, data: UpdateAgentRequest\)/)
  assert.match(skillAPI, /`\$\{platformSkillPath\(tenantId\)\}\/installer-agent`/)
  assert.doesNotMatch(skillAPI, /\/api\/v1\/agents/)
})

test('platform skill panels do not call the enterprise agent API', () => {
  for (const source of [skillSettings, sandboxSkillsPanel]) {
    assert.match(source, /getSkillInstallerAgent\(platformTenantID\.value!\)/)
    assert.match(source, /updateSkillInstallerAgent\(platformTenantID\.value!,/)
    assert.doesNotMatch(source, /\bgetAgentById\b/)
    assert.doesNotMatch(source, /\bupdateAgent\b/)
    assert.doesNotMatch(source, /builtin-skill-installer/)
  }
})
