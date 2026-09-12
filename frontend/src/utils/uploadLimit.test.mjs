import assert from 'node:assert/strict'
import test from 'node:test'

import { isSkillBundleUploadUrl } from './uploadLimit.ts'

test('skill catalog and sandbox skill uploads use the skill-bundle 413 path', () => {
  const base = '/api/v1/system/admin/tenants/42'
  assert.equal(isSkillBundleUploadUrl(`${base}/skills/catalog`), true)
  assert.equal(isSkillBundleUploadUrl(`${base}/skills/catalog/`), true)
  assert.equal(isSkillBundleUploadUrl(`${base}/skills/catalog?foo=1`), true)
  assert.equal(isSkillBundleUploadUrl(`${base}/sandbox-configs/cfg-a/skills`), true)
  assert.equal(isSkillBundleUploadUrl(`https://host${base}/sandbox-configs/cfg-a/skills`), true)
})

test('skill JSON subpaths stay on the knowledge-size 413 path', () => {
  const base = '/api/v1/system/admin/tenants/42'
  assert.equal(isSkillBundleUploadUrl(`${base}/skills/catalog/cat-1/install`), false)
  assert.equal(isSkillBundleUploadUrl(`${base}/skills/catalog/cat-1`), false)
  assert.equal(isSkillBundleUploadUrl(`${base}/sandbox-configs/cfg-a/skills/sk-1`), false)
  assert.equal(isSkillBundleUploadUrl(`${base}/sandbox-configs/cfg-a/skills/sk-1/reinstall`), false)
  assert.equal(isSkillBundleUploadUrl('/api/v1/skills/catalog'), false)
  assert.equal(isSkillBundleUploadUrl('/api/v1/sandbox-configs/cfg-a/skills'), false)
})

test('knowledge and other API uploads stay on the knowledge-size 413 path', () => {
  assert.equal(isSkillBundleUploadUrl(undefined), false)
  assert.equal(isSkillBundleUploadUrl('/api/v1/knowledge'), false)
  assert.equal(isSkillBundleUploadUrl('/api/v1/knowledge-bases/kb-1/knowledge'), false)
  assert.equal(isSkillBundleUploadUrl('/api/v1/models/debug'), false)
})
