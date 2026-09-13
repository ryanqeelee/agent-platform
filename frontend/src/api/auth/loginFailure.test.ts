import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./index.ts', import.meta.url), 'utf8')

test('login localizes retryable service-unavailable failures', () => {
  assert.match(
    source,
    /error\?\.status === 503[\s\S]*?return t\('auth\.loginErrorRetry'\)/,
  )
})

test('login preserves invalid-credential messages', () => {
  assert.match(
    source,
    /return error\?\.message \|\| t\('error\.auth\.loginFailed'\)/,
  )
})
