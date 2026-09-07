import assert from 'node:assert/strict'
import test from 'node:test'
import { loadOperatingClient, OPERATING_CLIENT_MODULE_PATH } from './operatingClient.ts'

test('loads a module exposing the frozen controller factory', async () => {
  const createController = () => ({}) as never
  const module = await loadOperatingClient(async () => ({ createController }))
  assert.equal(module.createController, createController)
  assert.equal(OPERATING_CLIENT_MODULE_PATH, '/app/operating-client.js')
})

test('turns a missing runtime module into a recoverable loading error', async () => {
  await assert.rejects(
    loadOperatingClient(async () => { throw new TypeError('module not found') }),
    /经营分析组件加载失败，请重试/,
  )
})

test('rejects a loaded module without the controller factory', async () => {
  await assert.rejects(
    loadOperatingClient(async () => ({ default: {} })),
    /经营分析组件版本不兼容，请刷新后重试/,
  )
})
