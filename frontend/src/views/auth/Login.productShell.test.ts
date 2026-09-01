import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'
import { getProductShellLoginCopy } from '../../config/productShellBrand.ts'

test('login renders the ProductShellBrandV1 surface without upstream capability marketing', () => {
  const source = readFileSync(fileURLToPath(new URL('./Login.vue', import.meta.url)), 'utf8')
  const template = source.match(/<template>([\s\S]*?)<\/template>/)?.[1] || ''

  assert.match(template, /productShellBrand\.name/)
  assert.match(template, /loginCopy\.headline/)
  assert.match(template, /auth\.loginTitle/)
  assert.doesNotMatch(template, /capability-list/)
  assert.doesNotMatch(template, /loginCopy\.(employeeAssistant|operatingAnalysis)/)
  assert.doesNotMatch(template, /\$t\('platform\.(rag|wiki|agent|hybridSearch)/)
  assert.doesNotMatch(template, /WeKnora/i)
  assert.match(source, /@media \(prefers-reduced-motion: reduce\)/)
})

test('login copy stays platform-led across every supported locale', () => {
  for (const locale of ['zh-CN', 'en-US', 'ru-RU', 'ko-KR']) {
    const copy = getProductShellLoginCopy(locale)
    assert.ok(copy.eyebrow)
    assert.ok(copy.headline)
    assert.ok(copy.description)
  }
})
