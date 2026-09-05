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

test('employee-assistant handoff carries only an opaque reference across routes', () => {
  const api = readFileSync(new URL('./operatingAnalysis.ts', import.meta.url), 'utf8')
  const router = readFileSync(new URL('../router/index.ts', import.meta.url), 'utf8')
  const message = readFileSync(new URL('../views/chat/components/usermsg.vue', import.meta.url), 'utf8')

  assert.match(api, /post\('\/api\/v1\/operating-analysis-handoffs', \{\s*sourceSessionId,\s*sourceMessageId,\s*\}\)/)
  assert.match(api, /\/api\/auth\/operating-analysis-handoffs\/\$\{encodeURIComponent\(handoffRef\)\}\/consume/)
  assert.match(router, /sessionStorage\.setItem\(\s*OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY/)
  assert.match(router, /window\.location\.assign\(handoffPrompt \? '\/app\/\?data_surface=analysis&data_workspace=data' : `\/app\/\?data_surface=\$\{surface\}`\)/)
  assert.doesNotMatch(router, /[?&](?:question|prompt)=/)
  assert.match(message, /emit\('handoff', messageId\)/)
})
