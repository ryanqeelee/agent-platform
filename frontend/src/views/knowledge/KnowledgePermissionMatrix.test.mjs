import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const listPath = new URL('./KnowledgeBaseList.vue', import.meta.url)
const detailPath = new URL('./KnowledgeBase.vue', import.meta.url)
const faqPath = new URL('./components/FAQEntryManager.vue', import.meta.url)
const scopeDialogPath = new URL('./components/KnowledgeAccessScopeDialog.vue', import.meta.url)
const editorModalPath = new URL('./KnowledgeBaseEditorModal.vue', import.meta.url)

test('knowledge list lifecycle is Admin+ while creation is Knowledge Administrator+', async () => {
  const source = await readFile(listPath, 'utf8')
  assert.match(source, /v-if="authStore\.hasRole\('admin'\)"[^>]*@click="handleCreateKnowledgeBase"/s)
  assert.match(source, /const handleCreateKnowledgeBase = \(\) => \{\s*if \(!authStore\.hasRole\('admin'\)\) return/s)
  assert.match(source, /uiStore\.openCreateKB\('document'\)/)
  assert.doesNotMatch(source, /openCreateKB\('document', initialSection\)/)
  assert.match(source, /function canManageKBCard\(kb: KB\): boolean \{\s*return authStore\.hasRole\('admin'\) && \(kb as any\)\.isMine !== false && \(kb as any\)\.permission == null/s)
  assert.match(source, /function canDuplicateKBCard\(kb: any\): boolean \{\s*return authStore\.hasRole\('admin'\) && kb\.isMine !== false && kb\.permission == null/s)
  assert.match(source, /const handleSettings = \(kb: KB\) => \{\s*if \(!canManageKBCard\(kb\)\) return/s)
  assert.match(source, /const handleDuplicate = async \(kb: KB\) => \{\s*if \(!canDuplicateKBCard\(kb\)\) return/s)
  assert.doesNotMatch(source, /if \(kb\.creator_id && userId && kb\.creator_id === userId\) return true/)
})

test('FAQ permission predicates contain no creator authority and preserve shared-role constraints', async () => {
  const source = await readFile(faqPath, 'utf8')
  assert.doesNotMatch(source, /const isOwner = computed/)
  assert.match(source, /if \(isViaShare\.value\) return authStore\.hasRole\('admin'\) && orgStore\.canEditKB\(props\.kbId, false\)/)
  assert.match(source, /return authStore\.hasRole\('admin'\)/)
  assert.match(source, /if \(isViaShare\.value\) return authStore\.hasRole\('admin'\) && orgStore\.canManageKB\(props\.kbId, false\)/)
  assert.match(source, /return authStore\.hasRole\('admin'\)/)
  assert.match(source, /const handleOpenKBSettings = \(\) => \{\s*if \(!canManage\.value\) return/s)
  assert.match(source, /<t-popup v-if="canEdit" v-model="entry\.showMore"/)
  assert.match(source, /if \(!canEdit\.value\) return\s*entry\.showMore = false\s*try \{\s*await deleteFAQEntries/s)
})

test('knowledge detail shared mutations require local Admin plus share authority', async () => {
  const source = await readFile(detailPath, 'utf8')
  assert.match(source, /if \(isViaShare\.value\) return authStore\.hasRole\('admin'\) && orgStore\.canEditKB\(kbId\.value, false\)/)
  assert.match(source, /if \(isViaShare\.value\) return authStore\.hasRole\('admin'\) && orgStore\.canManageKB\(kbId\.value, false\)/)
  assert.match(source, /const canMutateKnowledge = computed\(\(\) => \{\s*if \(!canEdit\.value\) return false;\s*if \(isViaShare\.value\) return authStore\.hasRole\('admin'\)/s)
})

test('knowledge settings share and activity controls ignore creator identity', async () => {
  const source = await readFile(editorModalPath, 'utf8')
  assert.match(source, /Number\(kbTenantId\.value \|\| 0\) !== Number\(authStore\.currentTenantId \|\| 0\)/)
  assert.match(source, /return authStore\.hasRole\('admin'\)/)
  assert.match(source, /const canViewActivity = computed\(\(\) => \{\s*return canShareKB\.value/s)
  assert.doesNotMatch(source, /kbCreatorId\.value === userId/)
})

test('knowledge access scope dialog cannot save stale defaults before both authorities load', async () => {
  const source = await readFile(scopeDialogPath, 'utf8')
  assert.match(source, /const loaded = ref\(false\)/)
  assert.match(source, /disabled: !loaded \|\| loading/)
  assert.match(source, /if \(!loaded\.value \|\| loading\.value\) return/)
  assert.match(source, /watch\(\(\) => \[props\.visible, props\.kbId\],[\s\S]*roles\.value = \[\]\s*roleIDs\.value = \[\]\s*mode\.value = 'all'\s*loaded\.value = false\s*void load\(\)/)
  assert.match(source, /Promise\.all\(\[listBusinessRoles\(\), getKnowledgeAccessScope\(props\.kbId\)\]\)[\s\S]*loaded\.value = true/)
  assert.match(source, /catch \{\s*if \(version === loadVersion && props\.visible\) MessagePlugin\.error/s)
})
