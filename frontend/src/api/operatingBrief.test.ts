import assert from 'node:assert/strict'
import test from 'node:test'
import { createOperatingBriefClient } from './operatingBrief.ts'

test('each brief call exchanges a token and sends the current Platform actor binding', async () => {
  const requests: Array<{ path: string; init: RequestInit }> = []
  let exchangeCalls = 0
  let credentials = { token: 'platform-one', tenantId: 'tenant-one' }
  const client = createOperatingBriefClient({
    exchange: async () => ({ access_token: `center-${++exchangeCalls}`, expires_in: 900 }),
    readPlatformCredentials: () => credentials,
    fetch: async (path, init = {}) => {
      requests.push({ path: String(path), init })
      return Response.json({ contractVersion: 'operating-brief/2' })
    },
  })

  await client.getOperatingBrief('scope / one')
  credentials = { token: 'platform-two', tenantId: 'tenant-two' }
  await client.getOperatingBrief(null)

  assert.equal(exchangeCalls, 2)
  assert.equal(requests[0].path, '/api/agents/data/operating-brief?scopeRef=scope+%2F+one&preparedOnly=true')
  assert.equal(requests[1].path, '/api/agents/data/operating-brief?preparedOnly=true')
  const firstHeaders = new Headers(requests[0].init.headers)
  const secondHeaders = new Headers(requests[1].init.headers)
  assert.equal(firstHeaders.get('authorization'), 'Bearer center-1')
  assert.equal(firstHeaders.get('x-platform-authorization'), 'Bearer platform-one')
  assert.equal(firstHeaders.get('x-platform-tenant-id'), 'tenant-one')
  assert.equal(secondHeaders.get('authorization'), 'Bearer center-2')
  assert.equal(secondHeaders.get('x-platform-authorization'), 'Bearer platform-two')
  assert.equal(secondHeaders.get('x-platform-tenant-id'), 'tenant-two')
})

test('handoff posts only the verified snapshot and observation references', async () => {
  let captured: { path: string; init: RequestInit } | undefined
  const client = createOperatingBriefClient({
    exchange: async () => ({ access_token: 'center', expires_in: 900 }),
    readPlatformCredentials: () => ({ token: 'platform', tenantId: 'tenant' }),
    fetch: async (path, init = {}) => {
      captured = { path: String(path), init }
      return Response.json({ schema: 'OperatingAnalysisHandoffV1', question: '服务端完整问题' })
    },
  })

  const result = await client.createOperatingBriefAnalysisHandoff({
    briefSnapshotRef: 'brief-ref',
    observationAnchorRef: 'anchor-ref',
  })

  assert.equal(captured?.path, '/api/agents/data/operating-brief/analysis-handoff')
  assert.equal(captured?.init.method, 'POST')
  assert.equal(captured?.init.body, JSON.stringify({
    briefSnapshotRef: 'brief-ref',
    observationAnchorRef: 'anchor-ref',
  }))
  assert.deepEqual(result, { schema: 'OperatingAnalysisHandoffV1', question: '服务端完整问题' })
})

test('brief requests retain the configured application base path', async () => {
  let path = ''
  const client = createOperatingBriefClient({
    exchange: async () => ({ access_token: 'center', expires_in: 900 }),
    readPlatformCredentials: () => ({ token: 'platform', tenantId: 'tenant' }),
    resolveBaseUrl: () => '/app/weknora',
    fetch: async input => {
      path = String(input)
      return Response.json({ contractVersion: 'operating-brief/2' })
    },
  })

  await client.getOperatingBrief(null)
  assert.equal(path, '/app/weknora/api/agents/data/operating-brief?preparedOnly=true')
})

test('an actor invalidation during token exchange prevents the Center data call', async () => {
  let finishExchange!: (value: { access_token: string; expires_in: number }) => void
  let exchangeSignal: AbortSignal | undefined
  let fetchCalls = 0
  const client = createOperatingBriefClient({
    exchange: signal => {
      exchangeSignal = signal
      return new Promise(resolve => { finishExchange = resolve })
    },
    readPlatformCredentials: () => ({ token: 'platform', tenantId: 'tenant' }),
    fetch: async () => {
      fetchCalls += 1
      return Response.json({})
    },
  })
  const controller = new AbortController()
  const request = client.getOperatingBrief(null, controller.signal)

  controller.abort()
  assert.equal(exchangeSignal?.aborted, true)
  finishExchange({ access_token: 'stale-center-token', expires_in: 900 })

  await assert.rejects(request, error => error instanceof DOMException && error.name === 'AbortError')
  assert.equal(fetchCalls, 0)
})
