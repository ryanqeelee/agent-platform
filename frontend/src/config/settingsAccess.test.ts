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

test('the skill catalog and sandbox require platform administration', () => {
  assert.equal(SETTINGS_SECTION_MIN_ROLE.skills, 'admin')
  assert.equal(SETTINGS_SECTION_MIN_ROLE.skills, SETTINGS_SECTION_MIN_ROLE.sandbox)
  assert.equal(SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE.skills, 'admin')
  assert.equal(SYSTEM_ADMIN_SETTINGS_SECTIONS.has('skills'), true)
  assert.equal(SYSTEM_ADMIN_SETTINGS_SECTIONS.has('sandbox'), true)
})

test('personal skill environment variables are visible to every member', () => {
  assert.equal(SETTINGS_SECTION_MIN_ROLE.envvars, 'viewer')
  // Workspace-wide skill env values live on the Admin+ skills page; a
  // management shortcut on the avatar menu would only duplicate that entrance.
  assert.equal(
    Object.prototype.hasOwnProperty.call(SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE, 'envvars'),
    false,
  )
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
      'sandbox',
      'skills',
      'system-global',
      'runtime-queues',
      'platform-api-keys',
      'system-audit-log',
    ],
  )
})
