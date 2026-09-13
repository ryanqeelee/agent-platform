import assert from 'node:assert/strict'
import test from 'node:test'

import {
  activationPayload,
  createActivationCommand,
  enterpriseFormDraft,
  enterpriseUpdatePayload,
  freshEnterpriseCreationDraft,
  GIB_BYTES,
  initialAdministratorCommand,
} from './platformOperationsModel'

test('enterprise activation uses the frozen browser DTO and stable command identifiers', () => {
  const payload = activationPayload({
    name: '  示例企业 ',
    description: '  说明 ',
    seats_total: 12,
    storage_quota_gib: 20,
  }, 'user-owner')
  const ids = ['activation-id', 'idempotency-key']
  const command = createActivationCommand(payload, () => ids.shift()!)

  assert.deepEqual(command, {
    activationId: 'activation-id',
    idempotencyKey: 'idempotency-key',
    payload: {
      name: '示例企业',
      description: '说明',
      seats_total: 12,
      storage_quota: 20 * GIB_BYTES,
      initial_administrator_user_id: 'user-owner',
    },
  })
})

test('enterprise edit preserves an unlimited seat quota', () => {
  const draft = enterpriseFormDraft({
    id: 1,
    name: '不限席位企业',
    description: '',
    status: 'active',
    analysis_enabled: false,
    seats_total: null,
    seats_used: 7,
    storage_quota: GIB_BYTES,
  })

  assert.equal(draft.seats_total, null)
})

test('enterprise update always sends an explicit seat quota', () => {
  const base = {
    name: ' Acme ',
    description: ' Retail ',
    status: 'active' as const,
    analysis_enabled: false,
    storage_quota_gib: 1,
  }

  assert.deepEqual(enterpriseUpdatePayload({ ...base, seats_total: undefined }), {
    name: 'Acme',
    description: 'Retail',
    status: 'active',
    analysis_enabled: false,
    seats_total: null,
    storage_quota: GIB_BYTES,
  })
  assert.equal(enterpriseUpdatePayload({ ...base, seats_total: null }).seats_total, null)
  assert.equal(enterpriseUpdatePayload({ ...base, seats_total: 0 }).seats_total, 0)
  assert.equal(enterpriseUpdatePayload({ ...base, seats_total: 7 }).seats_total, 7)
})

test('enterprise edit and activation preserve an unlimited storage quota', () => {
  const draft = enterpriseFormDraft({
    id: 1,
    name: '不限存储企业',
    description: '',
    status: 'active',
    analysis_enabled: false,
    seats_total: 3,
    seats_used: 1,
    storage_quota: 0,
  })

  assert.equal(draft.storage_quota_gib, 0)
  assert.equal(activationPayload({ ...draft, seats_total: 3 }, 'user-owner').storage_quota, 0)
})

test('initial administrator command is stable for one identity and contains no password', () => {
  const first = initialAdministratorCommand(null, ' admin ', ' admin@example.com ', () => 'command-1')
  const replay = initialAdministratorCommand(first, 'admin', 'admin@example.com', () => 'command-2')
  const changed = initialAdministratorCommand(first, 'other', 'other@example.com', () => 'command-3')

  assert.equal(replay, first)
  assert.equal(changed.commandId, 'command-3')
  assert.deepEqual(Object.keys(first).sort(), ['commandId', 'email', 'username'])
})

test('the recoverable activation command never contains the administrator password', () => {
  const form = {
    name: '示例企业',
    description: '',
    seats_total: 3,
    storage_quota_gib: 5,
    password: 'must-not-be-persisted',
  }
  const command = createActivationCommand(activationPayload(form, 'user-owner'), () => 'stable-id')

  assert.equal(JSON.stringify(command).includes(form.password), false)
  assert.deepEqual(Object.keys(command.payload).sort(), [
    'description',
    'initial_administrator_user_id',
    'name',
    'seats_total',
    'storage_quota',
  ])
})

test('a fresh creation draft cannot inherit a failed activation identity', () => {
  assert.deepEqual(freshEnterpriseCreationDraft(), {
    name: '',
    description: '',
    seats_total: 1,
    storage_quota_gib: 10,
    username: '',
    email: '',
    password: '',
  })
})
