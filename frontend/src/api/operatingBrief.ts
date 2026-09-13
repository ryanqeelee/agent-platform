import { getApiBaseUrl } from '../utils/api-base'

export interface OperatingBriefCoverageDTO {
  status: string
  expectedStoreCount: number | null
  coveredStoreCount: number | null
  observedStoreCount: number | null
  authoritativeZeroStoreCount: number | null
  missingStoreCount: number | null
  coverageRate: string | null
}

export interface OperatingBriefComparisonDTO {
  status: string
  value: string | null
  unit: string | null
  observationAnchorRef: string | null
}

export interface OperatingBriefPeriodDTO {
  start: string | null
  endExclusive: string | null
  dataFinality: string | null
  readyAt: string | null
  status: string
  coverage: OperatingBriefCoverageDTO
}

export interface OperatingBriefKpiDTO {
  key: string
  label: string
  value: string | null
  unit: string | null
  status: string
  yoy: Record<string, OperatingBriefComparisonDTO>
  wow: Record<string, OperatingBriefComparisonDTO>
  observationAnchorRef: string | null
}

export interface OperatingBriefDataDTO {
  periods: Record<string, OperatingBriefPeriodDTO>
  kpis: OperatingBriefKpiDTO[]
  salesTrend: Array<{
    start: string | null
    endExclusive: string | null
    status: string
    coverage: OperatingBriefCoverageDTO
    sales: { status: string; value: string | null; unit: string | null }
    observationAnchorRef: string | null
  }>
  salesYoyImpact: {
    status: 'available' | 'unavailable'
    items?: Array<{
      label: string
      amount: string | null
      rollup: boolean
      observationAnchorRef: string | null
    }>
  }
  focusItems: Array<{
    key: string
    statement: string
    observationAnchorRef: string | null
  }>
  suggestedQuestions: Array<{
    key: string
    question: string
    reason?: string
    observationAnchorRef: string | null
  }>
}

export interface OperatingBriefDTO {
  contractVersion: 'operating-brief/2'
  displayState: 'preparing' | 'no_data' | 'partial' | 'ready' | 'unavailable'
  generatedAt: string
  referenceExpiresAt: string | null
  selectedScope: {
    kind: 'all_operating_stores' | 'store'
    label: string
    scopeRef: string | null
  }
  scopeOptions: Array<{
    kind: 'all_operating_stores' | 'store'
    label: string
    scopeRef: string | null
  }>
  weeklyCore: {
    status: 'preparing' | 'no_data' | 'ready' | 'unavailable'
    completeness: 'complete' | 'partial' | null
    reasonCode: string | null
    pollable: boolean
    retryAfterSeconds: number | null
    inputSetDigest: string | null
    revision: number | null
    readyAt: string | null
    data: OperatingBriefDataDTO | null
  }
  inventory: {
    status: 'not_applicable'
    reasonCode: 'capability_not_available'
    pollable: false
    observations: []
  }
  notices: Array<{ level?: string; message?: string }>
  briefSnapshotRef: string | null
}

export interface OperatingBriefAnalysisHandoffDTO {
  schema: 'OperatingAnalysisHandoffV1'
  question: string
}

export interface OperatingBriefAnalysisHandoffInput {
  briefSnapshotRef: string
  observationAnchorRef: string
}

export class OperatingBriefRequestError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message)
    this.name = 'OperatingBriefRequestError'
  }
}

type Fetch = typeof globalThis.fetch

interface OperatingBriefClientDependencies {
  fetch: Fetch
  readPlatformCredentials: () => { token: string; tenantId: string }
  resolveBaseUrl?: () => string
}

export interface OperatingBriefClient {
  getOperatingBrief(scopeRef?: string | null, signal?: AbortSignal): Promise<OperatingBriefDTO>
	refreshOperatingBrief(scopeRef?: string | null, signal?: AbortSignal): Promise<void>
  createOperatingBriefAnalysisHandoff(
    input: OperatingBriefAnalysisHandoffInput,
    signal?: AbortSignal,
  ): Promise<OperatingBriefAnalysisHandoffDTO>
}

function abortIfNeeded(signal?: AbortSignal) {
  if (!signal?.aborted) return
  throw new DOMException('The operation was aborted.', 'AbortError')
}

async function responseError(response: Response): Promise<OperatingBriefRequestError> {
  let message = String(response.status)
  try {
    const body = await response.json() as { detail?: unknown; message?: unknown }
    if (typeof body.detail === 'string') message = body.detail
    else if (typeof body.message === 'string') message = body.message
  } catch {
    // Status remains the stable decision input when the server has no JSON body.
  }
  return new OperatingBriefRequestError(message, response.status)
}

export function createOperatingBriefClient(
  dependencies: OperatingBriefClientDependencies,
): OperatingBriefClient {
  async function request<T>(path: string, init: RequestInit, signal?: AbortSignal): Promise<T> {
    abortIfNeeded(signal)
    const platform = dependencies.readPlatformCredentials()
    const headers = new Headers(init.headers)
	if (platform.token) headers.set('authorization', `Bearer ${platform.token}`)
    if (platform.token && platform.tenantId) {
		headers.set('x-tenant-id', platform.tenantId)
    }

    const response = await dependencies.fetch(`${dependencies.resolveBaseUrl?.() ?? ''}${path}`, {
      ...init,
      cache: 'no-store',
      headers,
      signal,
    })
    if (!response.ok) throw await responseError(response)
    return await response.json() as T
  }

  return {
    getOperatingBrief(scopeRef, signal) {
      const params = new URLSearchParams()
      if (scopeRef) params.set('scopeRef', scopeRef)
      return request<OperatingBriefDTO>(
		`/api/v1/operating-brief${params.size ? `?${params.toString()}` : ''}`,
        { method: 'GET' },
        signal,
      )
    },

	async refreshOperatingBrief(scopeRef, signal) {
		const params = new URLSearchParams()
		if (scopeRef) params.set('scopeRef', scopeRef)
		await request<void>(
			`/api/v1/operating-brief/refresh${params.size ? `?${params.toString()}` : ''}`,
			{ method: 'POST' },
			signal,
		)
	},

    createOperatingBriefAnalysisHandoff(input, signal) {
      return request<OperatingBriefAnalysisHandoffDTO>(
		'/api/v1/operating-brief/analysis-handoff',
        {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({
            briefSnapshotRef: input.briefSnapshotRef,
            observationAnchorRef: input.observationAnchorRef,
          }),
        },
        signal,
      )
    },
  }
}

export function createCurrentUserOperatingBriefClient(
  readPlatformCredentials: OperatingBriefClientDependencies['readPlatformCredentials'],
): OperatingBriefClient {
  return createOperatingBriefClient({
  fetch: (...args) => globalThis.fetch(...args),
  readPlatformCredentials,
  resolveBaseUrl: getApiBaseUrl,
})
}
