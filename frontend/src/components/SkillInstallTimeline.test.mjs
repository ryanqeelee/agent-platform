import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { reconstructEventStreamFromSteps } from '../utils/agent-event-history.ts'

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
  assert.match(timeline, /reconstructEventStreamFromSteps\(/)
  assert.doesNotMatch(timeline, /messages\.splice\(0, messages\.length, \.\.\.response\.data\)/)
})

test('durable installer messages reconstruct command, output, and final answer events', () => {
  const events = reconstructEventStreamFromSteps(
    [
      {
        iteration: 1,
        tool_calls: [
          {
            id: 'tool-1',
            name: 'shell',
            args: { command: 'python install.py' },
            result: { success: true, output: 'installed skill package' },
          },
        ],
      },
    ],
    'Installation completed.',
    true,
  )

  const command = events.find((event) => event.type === 'tool_call')
  assert.deepEqual(command?.arguments, { command: 'python install.py' })
  assert.equal(command?.output, 'installed skill package')
  assert.deepEqual(
    events.find((event) => event.type === 'answer'),
    { type: 'answer', content: 'Installation completed.', done: true },
  )
})

test('durable installer final answer remains visible without agent steps', () => {
  assert.deepEqual(
    reconstructEventStreamFromSteps([], 'Already installed.', true),
    [{ type: 'answer', content: 'Already installed.', done: true }],
  )
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
