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
})

test('employee viewers cannot deep-link into management surfaces', () => {
  assert.deepEqual(EMPLOYEE_SURFACE_MIN_ROLE, {
    enterpriseAdministration: 'contributor',
    knowledgeBases: 'contributor',
    agents: 'admin',
    organizations: 'admin',
  })
  assert.equal(employeeSurfaceMinRoleForPath('/platform/enterprise'), 'contributor')
  assert.equal(employeeSurfaceMinRoleForPath('/platform/knowledge-bases'), 'contributor')
  assert.equal(employeeSurfaceMinRoleForPath('/platform/knowledge-bases/kb-1'), undefined)
  assert.equal(employeeSurfaceMinRoleForPath('/platform/agents'), 'admin')
  assert.equal(employeeSurfaceMinRoleForPath('/platform/organizations'), 'admin')
  assert.equal(employeeSurfaceMinRoleForPath('/platform/creatChat'), undefined)
})

test('system administration settings stay explicitly system-admin-only', () => {
  assert.deepEqual(
    [...SYSTEM_ADMIN_SETTINGS_SECTIONS],
    [
      'models',
      'chathistory',
      'websearch',
      'parser',
	  'mcp',
      'ollama',
      'weknoracloud',
      'vectorstore',
      'storage',
      'system-global',
      'runtime-queues',
      'platform-api-keys',
      'system-audit-log',
    ],
  )
})
