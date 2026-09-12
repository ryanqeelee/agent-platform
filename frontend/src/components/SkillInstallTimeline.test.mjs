import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const timeline = readFileSync(new URL('./SkillInstallTimeline.vue', import.meta.url), 'utf8')
const systemAPI = readFileSync(new URL('../api/system/index.ts', import.meta.url), 'utf8')

test('completed skill installs use only durable scoped history', () => {
  assert.match(timeline, /getConfigSkillTranscriptHistory\(tenantId, props\.configId, props\.skillId\)/)
  const completedBranch = timeline.slice(
    timeline.indexOf('if (!props.live)'),
    timeline.indexOf('// Locators land after the installer sandbox is up.'),
  )
  assert.match(completedBranch, /await loadHistory\(run\)/)
  assert.doesNotMatch(completedBranch, /follow\(run\)/)
})

test('persistent history uses only the canonical platform tenant path', () => {
  assert.match(
    systemAPI,
    /platformSandboxConfigPath\(tenantId\).*skills\/\$\{skillId\}\/transcript\/history/s,
  )
  for (const source of [timeline, systemAPI]) {
    assert.doesNotMatch(source, /@\/api\/chat/)
    assert.doesNotMatch(source, /\/api\/v1\/sessions/)
    assert.doesNotMatch(source, /getMessageList/)
  }
})
