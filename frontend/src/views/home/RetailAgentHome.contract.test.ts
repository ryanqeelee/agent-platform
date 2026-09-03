import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const source = readFileSync(fileURLToPath(new URL('./RetailAgentHome.vue', import.meta.url)), 'utf8')
const copySource = readFileSync(fileURLToPath(new URL('../../config/productShellBrand.ts', import.meta.url)), 'utf8')
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
  assert.match(source, /'roadmap-future': index > 1/)
  assert.match(source, /:aria-disabled="index > 1 \? true : undefined"/)
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

test('home frames current work inside the durable retail operating loop', () => {
  assert.match(copySource, /让零售经营中的知识、数据与判断不再分散/)
  assert.match(copySource, /loopSteps: \['发现问题', '分析判断', '人工确认', '执行协同', '结果复盘'\]/)
  assert.match(copySource, /loopTitle: '一个产品，两种当前工作入口'/)
  assert.doesNotMatch(copySource, /能力开放说明|当前从员工助理|未来逐步进入/)
})
