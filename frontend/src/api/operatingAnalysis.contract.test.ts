import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

test('Center history has no product proxy or client', () => {
  const nginx = readFileSync(new URL('../../nginx.conf', import.meta.url), 'utf8')
  const api = readFileSync(new URL('./operatingAnalysis.ts', import.meta.url), 'utf8')
  assert.doesNotMatch(nginx + api, /operating-analysis-history|OperatingAnalysisRevocationHistory/)
})

test('employee-assistant handoff carries only an opaque reference across routes', () => {
  const api = readFileSync(new URL('./operatingAnalysis.ts', import.meta.url), 'utf8')
  const router = readFileSync(new URL('../router/index.ts', import.meta.url), 'utf8')
  const message = readFileSync(new URL('../views/chat/components/usermsg.vue', import.meta.url), 'utf8')

  assert.match(api, /post\('\/api\/v1\/operating-analysis-handoffs', \{\s*sourceSessionId,\s*sourceMessageId,\s*\}\)/)
	assert.match(api, /\/api\/v1\/operating-analysis-handoffs\/\$\{encodeURIComponent\(handoffRef\)\}\/consume/)
  assert.match(router, /sessionStorage\.setItem\(\s*OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY/)
  assert.doesNotMatch(router, /window\.location\.assign\(handoffPrompt/)
  assert.doesNotMatch(router, /[?&](?:question|prompt)=/)
  assert.match(message, /emit\('handoff', messageId\)/)
})

test('native operating brief has no durable Center credential or app bundle bridge', () => {
  const operatingAnalysisApi = readFileSync(new URL('./operatingAnalysis.ts', import.meta.url), 'utf8')
  const briefApi = readFileSync(new URL('./operatingBrief.ts', import.meta.url), 'utf8')
  const briefView = readFileSync(new URL('../views/operating/OperatingBriefWorkspace.vue', import.meta.url), 'utf8')

	assert.match(briefApi, /\/api\/v1\/operating-brief/)
	assert.doesNotMatch(operatingAnalysisApi + briefApi, /exchangeOperatingAnalysis|weknora-exchange/)
  assert.doesNotMatch(operatingAnalysisApi, /retail_ai_app_auth_token|document\.cookie|localStorage\.setItem/)
  assert.doesNotMatch(briefApi, /retail_ai_app_auth_token|document\.cookie|localStorage\.setItem/)
  assert.doesNotMatch(briefView, /\/app\/operating-brief|mountOperatingBrief|document\.cookie|localStorage\.setItem/)
})

test('brief uses the effective current tenant, including the default tenant before a switch', () => {
  const view = readFileSync(new URL('../views/operating/OperatingBriefWorkspace.vue', import.meta.url), 'utf8')
  const api = readFileSync(new URL('./operatingBrief.ts', import.meta.url), 'utf8')
  assert.match(view, /tenantId: String\(auth\.effectiveTenantId \?\? ''\)/)
  assert.match(view, /token: auth\.token/)
  assert.doesNotMatch(view + api, /weknora_selected_tenant_id|auth\.selectedTenantId/)
})
