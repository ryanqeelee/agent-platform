import assert from 'node:assert/strict'
import test from 'node:test'

import {
  employeeSurfaceMinRoleForPath,
  EMPLOYEE_SURFACE_MIN_ROLE,
  SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE,
  SETTINGS_SECTION_MIN_ROLE,
  SYSTEM_ADMIN_SETTINGS_SECTIONS,
} from './settingsAccess'

test('management shortcuts are stricter than read-only settings pages', () => {
  assert.equal(SETTINGS_SECTION_MIN_ROLE.members, 'viewer')
  assert.equal(SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE.members, 'admin')
  assert.equal(SETTINGS_SECTION_MIN_ROLE.models, 'contributor')
  assert.equal(SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE.models, 'admin')
})

test('employee viewers cannot deep-link into management surfaces', () => {
  assert.deepEqual(EMPLOYEE_SURFACE_MIN_ROLE, {
    knowledgeBases: 'contributor',
    agents: 'contributor',
    organizations: 'admin',
  })
  assert.equal(employeeSurfaceMinRoleForPath('/platform/knowledge-bases'), 'contributor')
  assert.equal(employeeSurfaceMinRoleForPath('/platform/knowledge-bases/kb-1'), undefined)
  assert.equal(employeeSurfaceMinRoleForPath('/platform/agents'), 'contributor')
  assert.equal(employeeSurfaceMinRoleForPath('/platform/organizations'), 'admin')
  assert.equal(employeeSurfaceMinRoleForPath('/platform/creatChat'), undefined)
})

test('system administration settings stay explicitly system-admin-only', () => {
  assert.deepEqual(
    [...SYSTEM_ADMIN_SETTINGS_SECTIONS],
    ['system-global', 'runtime-queues', 'platform-api-keys', 'system-audit-log'],
  )
})
