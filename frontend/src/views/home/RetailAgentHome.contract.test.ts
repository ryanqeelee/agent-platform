import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const source = readFileSync(fileURLToPath(new URL('./RetailAgentHome.vue', import.meta.url)), 'utf8')
test('home reads the two current work cards from their existing authorities', () => {
  assert.match(source, /getSessionsList\(1, 1, 'web'\)/)
  assert.match(source, /getOperatingAnalysisHistory\(\)/)
  assert.match(source, /response\.availability\.state === 'hidden'[\s\S]*?\? 'absent'/)
  assert.match(source, /response\.availability\.canReadHistory \? response\.recentWork : undefined/)
  assert.match(source, /type AnalysisState = 'absent' \| 'disabled' \| 'enabled'/)
  assert.doesNotMatch(source, /analysisState\.value = 'unavailable'/)
  assert.match(source, /error\?\.status === 403 \? 'absent' : 'disabled'/)
})

test('home keeps future work non-interactive and the employee surface free of upstream branding', () => {
  assert.match(source, /class="roadmap-future" aria-disabled="true"/)
  assert.doesNotMatch(source, /WeKnora|RAG|model|provider/i)
  assert.match(source, /@media \(prefers-reduced-motion: reduce\)/)
})

test('home presents the two authorities as connected workspaces without merging their histories', () => {
  assert.match(source, /copy\.handoffHint/)
  assert.match(source, /copy\.employeeRecentWork/)
  assert.match(source, /copy\.analysisRecentWork/)
  assert.match(source, /assistant-green\.svg/)
  assert.match(source, /analysis-green\.svg/)
})
