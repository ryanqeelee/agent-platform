import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

test('fresh operating-analysis history is routed to the governed-analysis authority', () => {
  const nginx = readFileSync(new URL('../../nginx.conf', import.meta.url), 'utf8')

  assert.match(
    nginx,
    /location = \/api\/auth\/operating-analysis-history \{[\s\S]*?proxy_pass \$\{RETAIL_AI_CENTER_BASE_URL\}\/api\/auth\/operating-analysis-history;/,
  )
})
