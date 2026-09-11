import assert from 'node:assert/strict'
import test from 'node:test'
import type { OperatingBriefClient, OperatingBriefDTO } from '../../api/operatingBrief.ts'
import {
  createOperatingBriefController,
  createOperatingBriefState,
} from './operatingBriefController.ts'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { promise, resolve }
}

function brief(scopeRef: string | null, displayState: OperatingBriefDTO['displayState'] = 'ready'): OperatingBriefDTO {
  return {
    contractVersion: 'operating-brief/2',
    displayState,
    generatedAt: '2026-09-11T00:00:00Z',
    referenceExpiresAt: '2099-01-01T00:00:00Z',
    selectedScope: { kind: scopeRef ? 'store' : 'all_operating_stores', label: scopeRef ?? '全部', scopeRef },
    scopeOptions: [{ kind: 'all_operating_stores', label: '全部', scopeRef: null }],
    weeklyCore: {
      status: displayState === 'preparing' ? 'preparing' : 'ready',
      completeness: displayState === 'ready' ? 'complete' : null,
      reasonCode: null,
      pollable: displayState === 'preparing',
      retryAfterSeconds: displayState === 'preparing' ? 7 : null,
      inputSetDigest: null,
      revision: 1,
      readyAt: null,
      data: null,
    },
    inventory: { status: 'not_applicable', reasonCode: 'capability_not_available', pollable: false, observations: [] },
    notices: [],
    briefSnapshotRef: `snapshot-${scopeRef ?? 'all'}`,
  }
}

function source(overrides: Partial<OperatingBriefClient>): OperatingBriefClient {
  return {
    getOperatingBrief: async () => brief(null),
    createOperatingBriefAnalysisHandoff: async () => ({
      schema: 'OperatingAnalysisHandoffV1',
      question: 'server question',
    }),
    ...overrides,
  }
}

async function flush() {
  await Promise.resolve()
  await Promise.resolve()
}

test('actor reset clears prior data and scope/refresh races adopt only the latest request', async () => {
  const calls: Array<{
    scopeRef: string | null | undefined
    signal: AbortSignal | undefined
    result: ReturnType<typeof deferred<OperatingBriefDTO>>
  }> = []
  const state = createOperatingBriefState()
  const controller = createOperatingBriefController({
    state,
    source: source({
      getOperatingBrief: (scopeRef, signal) => {
        const result = deferred<OperatingBriefDTO>()
        calls.push({ scopeRef, signal, result })
        return result.promise
      },
    }),
    active: true,
    visible: true,
    onStartAnalysis() {},
    onOpenAnalysis() {},
  })

  const firstActor = controller.resetIdentity()
  assert.equal(calls.length, 1)
  state.brief = brief('old-visible')
  const secondActor = controller.resetIdentity()
  assert.equal(state.brief, null)
  assert.equal(calls[0].signal?.aborted, true)
  calls[1].result.resolve(brief('actor-two'))
  await secondActor
  calls[0].result.resolve(brief('actor-one-stale'))
  await firstActor
  assert.equal(state.selectedScopeRef, 'actor-two')

  const oldScope = controller.selectScope('store-old')
  const newScope = controller.selectScope('store-new')
  assert.equal(calls[2].signal?.aborted, true)
  assert.equal(calls[2].scopeRef, 'store-old')
  assert.equal(calls[3].scopeRef, 'store-new')
  calls[3].result.resolve(brief('store-new'))
  await newScope
  calls[2].result.resolve(brief('store-old'))
  await oldScope

  assert.equal(state.selectedScopeRef, 'store-new')
  controller.dispose()
})

test('prepared polling is bounded, honors retryAfter, and pauses while hidden', async () => {
  class FakeScheduler {
    tasks: Array<{ callback: () => void; delay: number; cancelled: boolean }> = []
    setTimeout(callback: () => void, delay: number) {
      const task = { callback, delay, cancelled: false }
      this.tasks.push(task)
      return task
    }
    clearTimeout(handle: unknown) {
      ;(handle as { cancelled: boolean }).cancelled = true
    }
    now() { return 0 }
    pending() { return this.tasks.filter(task => !task.cancelled) }
    runNext() {
      const task = this.pending()[0]
      assert.ok(task)
      task.cancelled = true
      task.callback()
    }
  }

  const scheduler = new FakeScheduler()
  let reads = 0
  const state = createOperatingBriefState()
  const controller = createOperatingBriefController({
    state,
    source: source({
      getOperatingBrief: async () => {
        reads += 1
        const value = brief(null, 'preparing')
        value.weeklyCore.retryAfterSeconds = reads === 1 ? 7 : 1
        return value
      },
    }),
    active: true,
    visible: true,
    scheduler,
    onStartAnalysis() {},
    onOpenAnalysis() {},
  })

  await controller.resetIdentity()
  assert.equal(scheduler.pending()[0]?.delay, 7000)
  controller.setActive(false)
  assert.equal(scheduler.pending().length, 0)
  controller.setActive(true)
  assert.equal(scheduler.pending()[0]?.delay, 7000)
  controller.setVisible(false)
  assert.equal(scheduler.pending().length, 0)
  controller.setVisible(true)
  assert.equal(scheduler.pending()[0]?.delay, 7000)

  const expectedDelays = [7000, 5000, 10000, 30000, 30000, 30000]
  for (const expected of expectedDelays) {
    assert.equal(scheduler.pending()[0]?.delay, expected)
    scheduler.runNext()
    await flush()
  }

  assert.equal(reads, 7)
  assert.equal(scheduler.pending().length, 0)
  assert.equal(state.pollExhausted, true)
  controller.dispose()
})

test('handoff is single-flight and cannot navigate after the brief loses ownership', async () => {
  const handoffs: Array<{
    input: { briefSnapshotRef: string; observationAnchorRef: string }
    signal: AbortSignal | undefined
    result: ReturnType<typeof deferred<{ schema: 'OperatingAnalysisHandoffV1'; question: string }>>
  }> = []
  const navigated: string[] = []
  const state = createOperatingBriefState()
  const controller = createOperatingBriefController({
    state,
    source: source({
      createOperatingBriefAnalysisHandoff: (input, signal) => {
        const result = deferred<{ schema: 'OperatingAnalysisHandoffV1'; question: string }>()
        handoffs.push({ input, signal, result })
        return result.promise
      },
    }),
    active: true,
    visible: true,
    onStartAnalysis: handoff => { navigated.push(handoff.question) },
    onOpenAnalysis() {},
  })

  await controller.resetIdentity()
  controller.ask('anchor-one', 'question one')
  const staleLaunch = controller.launch()
  void controller.launch()
  assert.equal(handoffs.length, 1)
  assert.deepEqual(handoffs[0].input, {
    briefSnapshotRef: 'snapshot-all',
    observationAnchorRef: 'anchor-one',
  })

  controller.setActive(false)
  assert.equal(handoffs[0].signal?.aborted, true)
  controller.setActive(true)
  handoffs[0].result.resolve({ schema: 'OperatingAnalysisHandoffV1', question: 'stale server question' })
  await staleLaunch
  assert.deepEqual(navigated, [])

  controller.ask('anchor-two', 'question two')
  const currentLaunch = controller.launch()
  handoffs[1].result.resolve({ schema: 'OperatingAnalysisHandoffV1', question: 'current server question' })
  await currentLaunch
  assert.deepEqual(navigated, ['current server question'])
  controller.dispose()
})
