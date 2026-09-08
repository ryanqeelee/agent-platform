import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { needsKnowledgeBaseConfiguration } from './knowledgeBaseInitialization'

// State x principal: redacted, incomplete, RAG, and non-RAG configurations.
const cases = [
  { name: 'redacted response', kb: {}, platformNeedsConfig: true },
  { name: 'missing summary', kb: { embedding_model_id: 'embed' }, platformNeedsConfig: true },
  { name: 'legacy RAG missing embedding', kb: { summary_model_id: 'chat' }, platformNeedsConfig: true },
  { name: 'vector indexing missing embedding', kb: { summary_model_id: 'chat', indexing_strategy: { vector_enabled: true } }, platformNeedsConfig: true },
  { name: 'keyword indexing missing embedding', kb: { summary_model_id: 'chat', indexing_strategy: { keyword_enabled: true } }, platformNeedsConfig: true },
  { name: 'configured RAG', kb: { summary_model_id: 'chat', embedding_model_id: 'embed' }, platformNeedsConfig: false },
  { name: 'non-RAG does not need embedding', kb: { summary_model_id: 'chat', indexing_strategy: { vector_enabled: false, keyword_enabled: false } }, platformNeedsConfig: false },
]
for (const scenario of cases) {
  for (const isSystemAdmin of [false, true]) {
    test(`${scenario.name}: ${isSystemAdmin ? 'platform' : 'enterprise'}`, () => {
      assert.equal(needsKnowledgeBaseConfiguration(scenario.kb, isSystemAdmin), isSystemAdmin && scenario.platformNeedsConfig)
    })
  }
}

for (const path of [
  '../views/knowledge/KnowledgeBaseList.vue',
  '../views/knowledge/KnowledgeBase.vue',
  '../views/platform/index.vue',
  '../components/KnowledgeBaseSelector.vue',
]) {
  test(`${path} uses the principal-aware configuration check`, () => {
    const source = readFileSync(new URL(path, import.meta.url), 'utf8')
    assert.match(source, /needsKnowledgeBaseConfiguration\([^,]+, authStore\.isSystemAdmin\)/)
    assert.doesNotMatch(source, /if \(!kb\.summary_model_id\)|k => k\.embedding_model_id && k\.summary_model_id/)
  })
}
