import assert from 'node:assert/strict'
import test from 'node:test'

import {
  normalizeModelEditorSource,
  shouldShowOllamaUnavailableTip,
  type ModelEditorType,
} from './modelEditorSourceState.ts'

test('platform model administration uses remote API sources for every model type', () => {
  const modelTypes: ModelEditorType[] = ['chat', 'embedding', 'rerank', 'vllm', 'asr']
  for (const modelType of modelTypes) {
    assert.equal(normalizeModelEditorSource('local', modelType, true), 'remote')
    assert.equal(normalizeModelEditorSource('remote', modelType, true), 'remote')
  }
})

test('enterprise model administration keeps Ollama for supported model types', () => {
  for (const modelType of ['chat', 'embedding', 'vllm', 'asr'] as const) {
    assert.equal(normalizeModelEditorSource('local', modelType, false), 'local')
  }
  assert.equal(normalizeModelEditorSource('local', 'rerank', false), 'remote')
})

test('hides Ollama unavailable tip while configuring a remote model', () => {
  assert.equal(shouldShowOllamaUnavailableTip('remote', 'chat', false), false)
})

test('shows Ollama unavailable tip only for local non-rerank models', () => {
  assert.equal(shouldShowOllamaUnavailableTip('local', 'chat', false), true)
  assert.equal(shouldShowOllamaUnavailableTip('local', 'embedding', false), true)
  assert.equal(shouldShowOllamaUnavailableTip('local', 'rerank', false), false)
})

test('does not show Ollama unavailable tip before status is known or when Ollama is available', () => {
  assert.equal(shouldShowOllamaUnavailableTip('local', 'chat', null), false)
  assert.equal(shouldShowOllamaUnavailableTip('local', 'chat', true), false)
})
