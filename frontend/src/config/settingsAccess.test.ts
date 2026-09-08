import assert from 'node:assert/strict'
import test from 'node:test'

import {
  employeeSurfaceMinRoleForPath,
  settingsSurfaceForSection,
  EMPLOYEE_SURFACE_MIN_ROLE,
  SETTINGS_SECTION_MIN_ROLE,
  SYSTEM_ADMIN_SETTINGS_SECTIONS,
} from './settingsAccess'

test('member management pages require enterprise administrators', () => {
  assert.equal(SETTINGS_SECTION_MIN_ROLE.members, 'admin')
})

test('employee viewers cannot deep-link into management surfaces', () => {
  assert.deepEqual(EMPLOYEE_SURFACE_MIN_ROLE, {
    enterpriseAdministration: 'admin',
    knowledgeBases: 'admin',
    agents: 'admin',
    organizations: 'admin',
  })
  assert.equal(employeeSurfaceMinRoleForPath('/platform/enterprise'), 'admin')
  assert.equal(employeeSurfaceMinRoleForPath('/platform/knowledge-bases'), 'admin')
  assert.equal(employeeSurfaceMinRoleForPath('/platform/knowledge-bases/kb-1'), undefined)
  assert.equal(employeeSurfaceMinRoleForPath('/platform/agents'), 'admin')
  assert.equal(employeeSurfaceMinRoleForPath('/platform/organizations'), 'admin')
  assert.equal(employeeSurfaceMinRoleForPath('/platform/creatChat'), undefined)
})

test('the skill catalog and sandbox require platform administration', () => {
  assert.equal(SETTINGS_SECTION_MIN_ROLE.skills, 'admin')
  assert.equal(SETTINGS_SECTION_MIN_ROLE.skills, SETTINGS_SECTION_MIN_ROLE.sandbox)
  assert.equal(SYSTEM_ADMIN_SETTINGS_SECTIONS.has('skills'), true)
  assert.equal(SYSTEM_ADMIN_SETTINGS_SECTIONS.has('sandbox'), true)
})

test('system administration settings stay explicitly system-admin-only', () => {
  assert.deepEqual(
    [...SYSTEM_ADMIN_SETTINGS_SECTIONS],
    [
      'agents',
      'memory-runtime',
      'diagnostics',
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

test('personal, enterprise and platform sections never share a settings surface', () => {
  for (const section of ['general', 'userprofile', 'mymemory', 'system']) {
    assert.equal(settingsSurfaceForSection(section), 'personal')
  }
  for (const section of ['tenant', 'members', 'businessRoles', 'memory', 'enterprise-skills']) {
    assert.equal(settingsSurfaceForSection(section), 'enterprise')
    assert.equal(SETTINGS_SECTION_MIN_ROLE[section], 'admin')
  }
  for (const section of SYSTEM_ADMIN_SETTINGS_SECTIONS) {
    assert.equal(settingsSurfaceForSection(section), 'platform')
  }
})
