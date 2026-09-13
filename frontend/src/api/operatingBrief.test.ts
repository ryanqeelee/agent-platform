import assert from 'node:assert/strict'
import test from 'node:test'
import { createOperatingBriefClient } from './operatingBrief.ts'

test('brief calls use the current Platform session and tenant directly', async () => {
  const requests: Array<{ path: string; init: RequestInit }> = []
  let credentials = { token: 'platform-one', tenantId: 'tenant-one' }
  const client = createOperatingBriefClient({
    readPlatformCredentials: () => credentials,
    fetch: async (path, init = {}) => {
      requests.push({ path: String(path), init })
      return Response.json({ contractVersion: 'operating-brief/2' })
    },
  })

  await client.getOperatingBrief('scope / one')
  credentials = { token: 'platform-two', tenantId: 'tenant-two' }
  await client.getOperatingBrief(null)

  assert.equal(requests[0].path, '/api/v1/operating-brief?scopeRef=scope+%2F+one')
  assert.equal(requests[1].path, '/api/v1/operating-brief')
  const firstHeaders = new Headers(requests[0].init.headers)
  const secondHeaders = new Headers(requests[1].init.headers)
  assert.equal(firstHeaders.get('authorization'), 'Bearer platform-one')
  assert.equal(firstHeaders.get('x-tenant-id'), 'tenant-one')
  assert.equal(secondHeaders.get('authorization'), 'Bearer platform-two')
  assert.equal(secondHeaders.get('x-tenant-id'), 'tenant-two')
})

test('manual refresh enqueues the selected Platform scope', async () => {
  let captured: { path: string; init: RequestInit } | undefined
  const client = createOperatingBriefClient({
    readPlatformCredentials: () => ({ token: 'platform', tenantId: 'tenant' }),
    fetch: async (path, init = {}) => {
      captured = { path: String(path), init }
      return Response.json({ status: 'queued' }, { status: 202 })
    },
  })

  await client.refreshOperatingBrief('store ref')

  assert.equal(captured?.path, '/api/v1/operating-brief/refresh?scopeRef=store+ref')
  assert.equal(captured?.init.method, 'POST')
})

test('handoff posts only the verified snapshot and observation references', async () => {
  let captured: { path: string; init: RequestInit } | undefined
  const client = createOperatingBriefClient({
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

  assert.equal(captured?.path, '/api/v1/operating-brief/analysis-handoff')
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
    readPlatformCredentials: () => ({ token: 'platform', tenantId: 'tenant' }),
    resolveBaseUrl: () => '/app/weknora',
    fetch: async input => {
      path = String(input)
      return Response.json({ contractVersion: 'operating-brief/2' })
    },
  })

  await client.getOperatingBrief(null)
  assert.equal(path, '/app/weknora/api/v1/operating-brief')
})
