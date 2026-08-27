import assert from 'node:assert/strict'
import test from 'node:test'

import {
  assignableMemberRoles,
  canManageMemberRole,
  canTransferMemberOwnership,
  tenantRoleTranslationKey,
} from './memberLifecycle'

test('member lifecycle UI matrix matches enterprise roles', () => {
  assert.equal(canManageMemberRole('owner', 'admin'), true)
  assert.equal(canManageMemberRole('owner', 'owner'), false)
  assert.equal(canManageMemberRole('admin', 'contributor'), true)
  assert.equal(canManageMemberRole('admin', 'admin'), false)
  assert.equal(canManageMemberRole('contributor', 'viewer'), false)
  assert.deepEqual(assignableMemberRoles('owner'), ['admin', 'contributor', 'viewer'])
  assert.deepEqual(assignableMemberRoles('admin'), ['contributor', 'viewer'])

  assert.equal(canManageMemberRole('admin', 'admin', false), false)
  assert.equal(canManageMemberRole('admin', 'admin', true), true)
  assert.equal(canManageMemberRole('admin', 'admin', true, true), true)
  assert.equal(canManageMemberRole('', 'owner', true), false)
  assert.deepEqual(assignableMemberRoles('', true), ['admin', 'contributor', 'viewer'])

  assert.equal(canTransferMemberOwnership('owner', 'admin', 'active', false), true)
  assert.equal(canTransferMemberOwnership('owner', 'admin', 'suspended', false), false)
  assert.equal(canTransferMemberOwnership('owner', 'admin', 'invited', false), false)
  assert.equal(canTransferMemberOwnership('admin', 'admin', 'active', true), false)

  assert.equal(tenantRoleTranslationKey('contributor'), 'tenantMember.role.contributor')
  assert.equal(tenantRoleTranslationKey('viewer'), 'tenantMember.role.viewer')
  assert.equal(tenantRoleTranslationKey('unknown'), '')
})
