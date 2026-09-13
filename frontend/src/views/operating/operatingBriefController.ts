import type {
  OperatingBriefAnalysisHandoffDTO,
  OperatingBriefClient,
  OperatingBriefDTO,
} from '@/api/operatingBrief'

export const OPERATING_BRIEF_POLL_DELAYS_SECONDS = [2, 5, 10, 30, 30, 30] as const

export interface OperatingBriefPendingQuestion {
  anchor: string
  question: string
}

export interface OperatingBriefState {
  brief: OperatingBriefDTO | null
  selectedScopeRef: string | null
  loading: boolean
  error: string
  pending: OperatingBriefPendingQuestion | null
  launching: boolean
  pollExhausted: boolean
}

interface Scheduler {
  setTimeout(callback: () => void, delayMilliseconds: number): unknown
  clearTimeout(handle: unknown): void
  now(): number
}

interface OperatingBriefControllerOptions {
  state: OperatingBriefState
  source: OperatingBriefClient
  active: boolean
  visible: boolean
  scheduler?: Scheduler
  onStartAnalysis(handoff: OperatingBriefAnalysisHandoffDTO): void | Promise<void>
  onOpenAnalysis(): void
}

interface ActiveBriefRequest {
  controller: AbortController
  sequence: number
  pollAttempt: number | null
}

export interface OperatingBriefController {
  refresh(): Promise<void>
  selectScope(scopeRef: string | null): Promise<void>
  resetIdentity(): Promise<void>
  setActive(active: boolean): void
  setVisible(visible: boolean): void
  ask(anchor: string | null, question: string): void
  cancelPending(): void
  launch(): Promise<void>
  openAnalysis(): void
  dispose(): void
}

const browserScheduler: Scheduler = {
  setTimeout: (callback, delay) => globalThis.setTimeout(callback, delay),
  clearTimeout: handle => globalThis.clearTimeout(handle as ReturnType<typeof setTimeout>),
  now: () => Date.now(),
}

export function createOperatingBriefState(): OperatingBriefState {
  return {
    brief: null,
    selectedScopeRef: null,
    loading: true,
    error: '',
    pending: null,
    launching: false,
    pollExhausted: false,
  }
}

export function operatingBriefErrorText(error: unknown): string {
  const status = typeof error === 'object' && error && 'status' in error
    ? (error as { status?: number }).status
    : undefined
  if (status === 409) return '简报范围已变化，请刷新后再试。'
  if (status === 503) return '暂时无法读取经营数据，请联系企业管理员检查数据连接与发布状态。'
  return '暂时无法加载经营简报，请重试。'
}

function isAbort(error: unknown): boolean {
  return typeof error === 'object' && error !== null && 'name' in error
    && (error as { name?: unknown }).name === 'AbortError'
}

export function createOperatingBriefController(
  options: OperatingBriefControllerOptions,
): OperatingBriefController {
  const scheduler = options.scheduler ?? browserScheduler
  const state = options.state
  let active = options.active
  let visible = options.visible
  let disposed = false
  let requestSequence = 0
  let activeRequest: ActiveBriefRequest | null = null
  let pollAttempt = 0
  let pendingPollTimer: unknown | null = null
  let ownerGeneration = 0
  let launchIntent = 0
  let launchStarted = false
  let handoffController: AbortController | null = null
	let refreshController: AbortController | null = null

  function clearPendingPollTimer() {
    if (pendingPollTimer === null) return
    scheduler.clearTimeout(pendingPollTimer)
    pendingPollTimer = null
  }

  function invalidateLaunch() {
    ownerGeneration += 1
    launchIntent += 1
    handoffController?.abort()
    handoffController = null
    launchStarted = false
    state.launching = false
    state.pending = null
  }

	function cancelRefresh() {
		refreshController?.abort()
		refreshController = null
	}

  function ownsRequest(request: ActiveBriefRequest): boolean {
    return !disposed
      && !request.controller.signal.aborted
      && request.sequence === requestSequence
  }

  function canPoll(brief: OperatingBriefDTO | null): brief is OperatingBriefDTO {
    return Boolean(
      active
      && visible
		&& brief?.weeklyCore.pollable,
    )
  }

  function schedulePolling() {
    clearPendingPollTimer()
    if (!canPoll(state.brief)) return
    if (pollAttempt >= OPERATING_BRIEF_POLL_DELAYS_SECONDS.length) {
      state.pollExhausted = true
      return
    }
    const attempt = pollAttempt
    const retryAfterSeconds = state.brief.weeklyCore.retryAfterSeconds ?? 0
    const delaySeconds = Math.max(OPERATING_BRIEF_POLL_DELAYS_SECONDS[attempt], retryAfterSeconds)
    const scheduledScopeRef = state.selectedScopeRef
    const timer = scheduler.setTimeout(() => {
      if (pendingPollTimer !== timer) return
      pendingPollTimer = null
      if (!active || !visible || state.selectedScopeRef !== scheduledScopeRef) return
      pollAttempt = attempt + 1
      void load(scheduledScopeRef, { pollAttempt: attempt })
    }, delaySeconds * 1000)
    pendingPollTimer = timer
  }

  async function load(
    scopeRef: string | null,
    behavior: { resetPolling?: boolean; pollAttempt?: number } = {},
  ): Promise<void> {
    if (disposed) return
    if (behavior.resetPolling) {
      clearPendingPollTimer()
      pollAttempt = 0
      state.pollExhausted = false
    }
    activeRequest?.controller.abort()
    const request: ActiveBriefRequest = {
      controller: new AbortController(),
      sequence: ++requestSequence,
      pollAttempt: behavior.pollAttempt ?? null,
    }
    activeRequest = request
    state.loading = true
    state.pending = null
    state.error = ''

    try {
      const value = await options.source.getOperatingBrief(scopeRef, request.controller.signal)
      if (!ownsRequest(request)) return
      state.brief = value
      state.selectedScopeRef = value.selectedScope.scopeRef
      launchStarted = false
    } catch (error) {
      if (!ownsRequest(request) || isAbort(error)) return
      state.error = operatingBriefErrorText(error)
    } finally {
      if (!ownsRequest(request)) return
      activeRequest = null
      state.loading = false
    }

    if (!state.error) schedulePolling()
  }

  function pausePollingRequest() {
    clearPendingPollTimer()
    const request = activeRequest
    if (!request || request.pollAttempt === null) return
    pollAttempt = request.pollAttempt
    request.controller.abort()
    requestSequence += 1
    activeRequest = null
    state.loading = false
  }

  return {
	async refresh() {
      if (!active || state.launching) return Promise.resolve()
		cancelRefresh()
		const controller = new AbortController()
		const generation = ownerGeneration
		refreshController = controller
		state.loading = true
		state.error = ''
		try {
			await options.source.refreshOperatingBrief(state.selectedScopeRef, controller.signal)
		} catch (error) {
			if (!isAbort(error) && generation === ownerGeneration) state.error = operatingBriefErrorText(error)
			return
		} finally {
			if (refreshController === controller) refreshController = null
			if (generation === ownerGeneration) state.loading = false
		}
		if (disposed || controller.signal.aborted || generation !== ownerGeneration) return
      return load(state.selectedScopeRef, { resetPolling: true })
    },

    selectScope(scopeRef) {
      if (!active || state.launching) return Promise.resolve()
      return load(scopeRef, { resetPolling: true })
    },

    resetIdentity() {
      clearPendingPollTimer()
		cancelRefresh()
      activeRequest?.controller.abort()
      activeRequest = null
      requestSequence += 1
      invalidateLaunch()
      pollAttempt = 0
      state.brief = null
      state.selectedScopeRef = null
      state.loading = active
      state.error = ''
      state.pollExhausted = false
      return active ? load(null, { resetPolling: true }) : Promise.resolve()
    },

    setActive(nextActive) {
      if (disposed || active === nextActive) return
      active = nextActive
      invalidateLaunch()
      if (!active) {
			cancelRefresh()
        pausePollingRequest()
        return
      }
      const expiresAt = state.brief?.referenceExpiresAt
      if (!state.brief || !expiresAt || Date.parse(expiresAt) <= scheduler.now()) {
        void load(state.selectedScopeRef, { resetPolling: true })
        return
      }
      schedulePolling()
    },

    setVisible(nextVisible) {
      if (disposed || visible === nextVisible) return
      visible = nextVisible
      if (!visible) {
        pausePollingRequest()
        return
      }
      schedulePolling()
    },

    ask(anchor, question) {
      if (!active || state.loading || state.error || !anchor || !state.brief?.briefSnapshotRef) return
      state.pending = { anchor, question }
    },

    cancelPending() {
      if (state.launching) return
      state.pending = null
    },

    async launch() {
      const briefSnapshotRef = state.brief?.briefSnapshotRef
      const pending = state.pending
      if (!active || state.loading || !briefSnapshotRef || !pending || state.launching || launchStarted) return

      const intent = ++launchIntent
      const generation = ownerGeneration
      const controller = new AbortController()
      handoffController = controller
      launchStarted = true
      state.launching = true
      const stillOwnsNavigation = () => (
        !disposed
        && active
        && !controller.signal.aborted
        && ownerGeneration === generation
        && launchIntent === intent
      )

      try {
        const handoff = await options.source.createOperatingBriefAnalysisHandoff({
          briefSnapshotRef,
          observationAnchorRef: pending.anchor,
        }, controller.signal)
        if (!stillOwnsNavigation()) return
        if (handoff.schema !== 'OperatingAnalysisHandoffV1' || !handoff.question.trim()) {
          throw new Error('invalid operating analysis handoff')
        }
        await options.onStartAnalysis(handoff)
      } catch (error) {
        if (!stillOwnsNavigation() || isAbort(error)) return
        launchStarted = false
        state.error = operatingBriefErrorText(error)
      } finally {
        if (handoffController === controller) handoffController = null
        if (launchIntent === intent) state.launching = false
      }
    },

    openAnalysis() {
      if (!active) return
      invalidateLaunch()
      options.onOpenAnalysis()
    },

    dispose() {
      if (disposed) return
      disposed = true
		cancelRefresh()
      active = false
      clearPendingPollTimer()
      activeRequest?.controller.abort()
      activeRequest = null
      requestSequence += 1
      invalidateLaunch()
    },
  }
}
