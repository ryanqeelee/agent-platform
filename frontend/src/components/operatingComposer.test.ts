import assert from 'node:assert/strict'
import test from 'node:test'
import {
  canSendOperatingDraft,
  isOperatingTerminalStatus,
  sendOperatingDraft,
  type OperatingComposer,
} from './operatingComposer.ts'

const createComposer = (overrides: Partial<OperatingComposer> = {}) => {
  const sent: string[] = []
  const composer: OperatingComposer = {
    draft: 'current question',
    disabled: false,
    running: false,
    cancelling: false,
    uploadPending: false,
    uploadAvailable: true,
    attachments: [],
    setDraft: () => undefined,
    send: text => sent.push(text),
    stop: () => undefined,
    upload: () => undefined,
    remove: () => undefined,
    ...overrides,
  }
  return { composer, sent }
}

test('operating send delegates the current text without clearing controlled draft', () => {
  const { composer, sent } = createComposer()

  assert.equal(sendOperatingDraft(composer), true)
  assert.deepEqual(sent, ['current question'])
  assert.equal(composer.draft, 'current question')
})

test('operating send stays blocked at each controlled unavailable state', () => {
  for (const overrides of [
    { draft: '   ' },
    { disabled: true },
    { running: true },
    { cancelling: true },
    { uploadPending: true },
  ]) {
    const { composer, sent } = createComposer(overrides)
    assert.equal(canSendOperatingDraft(composer), false)
    assert.equal(sendOperatingDraft(composer), false)
    assert.deepEqual(sent, [])
  }
})

test('only canonical terminal operating statuses complete the process timeline', () => {
  assert.equal(isOperatingTerminalStatus('idle'), false)
  assert.equal(isOperatingTerminalStatus('running'), false)
  assert.equal(isOperatingTerminalStatus('completed'), true)
  assert.equal(isOperatingTerminalStatus('cancelled'), true)
  assert.equal(isOperatingTerminalStatus('failed'), true)
  assert.equal(isOperatingTerminalStatus('incomplete'), true)
  assert.equal(isOperatingTerminalStatus(undefined), false)
})
