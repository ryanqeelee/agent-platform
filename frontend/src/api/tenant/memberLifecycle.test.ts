import assert from 'node:assert/strict'
import test from 'node:test'
import { assignableMemberRoles, canManageMemberRole, tenantRoleTranslationKey } from './memberLifecycle'
test('two enterprise roles and self-management boundary', () => {
  for (const target of ['admin', 'viewer'] as const) {
    assert.equal(canManageMemberRole('admin', target), true)
    assert.equal(canManageMemberRole('viewer', target), false)
    assert.equal(canManageMemberRole('admin', target, false, true), false)
  }
  assert.deepEqual(assignableMemberRoles('admin'), ['admin','viewer'])
  assert.deepEqual(assignableMemberRoles('viewer'), [])
  assert.deepEqual(assignableMemberRoles('', true), ['admin','viewer'])
  for (const retired of ['owner','contributor','unknown']) assert.equal(tenantRoleTranslationKey(retired),'')
  assert.equal(tenantRoleTranslationKey('viewer'),'tenantMember.role.viewer')
})
