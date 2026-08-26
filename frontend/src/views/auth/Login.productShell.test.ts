import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import { getProductShellLoginCopy } from '../../config/productShellBrand.ts'

test('login renders the ProductShellBrandV1 surface without upstream capability marketing', () => {
  const source = readFileSync(fileURLToPath(new URL('./Login.vue', import.meta.url)), 'utf8')
  const template = source.match(/<template>([\s\S]*?)<\/template>/)?.[1] || ''

  assert.match(template, /productShellBrand\.name/)
  assert.match(template, /loginCopy\.employeeAssistant/)
  assert.match(template, /loginCopy\.operatingAnalysis/)
  assert.match(template, /:aria-label="loginCopy\.capabilityListLabel"/)
  assert.doesNotMatch(template, /\$t\('platform\.(rag|wiki|agent|hybridSearch)/)
  assert.doesNotMatch(template, /WeKnora/i)
  assert.match(source, /@media \(prefers-reduced-motion: reduce\)/)
})

test('capability list labels are localized for every supported login locale', () => {
  const labels = ['zh-CN', 'en-US', 'ru-RU', 'ko-KR'].map(locale =>
    getProductShellLoginCopy(locale).capabilityListLabel,
  )
  assert.deepEqual(labels, ['产品能力', 'Product capabilities', 'Возможности продукта', '제품 기능'])
})
