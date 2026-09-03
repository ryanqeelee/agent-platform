import { get, post } from '@/utils/request'

export interface OperatingAnalysisAvailabilityV1 {
  schema: 'OperatingAnalysisAvailabilityV1'
  availability: {
    state: 'hidden' | 'disabled' | 'enabled'
    reasonCode: string
    canExchange: boolean
    canReadHistory: boolean
    nextAction: 'none' | 'contact_admin' | 'service_unavailable'
    revision?: string
    expiresAt?: string
  }
}

export interface OperatingAnalysisExchangeV1 {
  access_token: string
  expires_in: number
}

export interface OperatingAnalysisHandoffV1 {
  schema: 'OperatingAnalysisHandoffV1'
  handoffRef: string
  expiresAt: string
}

export interface OperatingAnalysisHandoffExchangeV1 extends OperatingAnalysisExchangeV1 {
  handoff: {
    schema: 'OperatingAnalysisHandoffV1'
    question: string
  }
}

export const OPERATING_ANALYSIS_HANDOFF_REF_KEY = 'operating_analysis_handoff_ref_v1'
export const OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY = 'operating_analysis_handoff_prompt_v1'
export const OPERATING_BRIEF_PREFETCH_KEY = 'operating_brief_prefetch_v1'

let operatingBriefPrefetch: Promise<void> | null = null

function cachedOperatingBriefIsFresh(): boolean {
  try {
    const cached = JSON.parse(sessionStorage.getItem(OPERATING_BRIEF_PREFETCH_KEY) || 'null')
    return cached?.brief?.contractVersion === 'operating-brief/1'
      && typeof cached?.brief?.expiresAt === 'string'
      && Date.parse(cached.brief.expiresAt) > Date.now()
  } catch {
    sessionStorage.removeItem(OPERATING_BRIEF_PREFETCH_KEY)
    return false
  }
}

// Warm the slow governed brief while the employee is already on the product shell. The cache
// stores only the public brief DTO; the short-lived Center token never enters browser storage.
export function prefetchOperatingBrief(): Promise<void> {
  if (cachedOperatingBriefIsFresh()) return Promise.resolve()
  if (operatingBriefPrefetch) return operatingBriefPrefetch
  operatingBriefPrefetch = exchangeOperatingAnalysis().then(async ({ access_token }) => {
    const response = await fetch('/api/agents/data/operating-brief', {
      headers: { Authorization: `Bearer ${access_token}` },
    })
    if (!response.ok) throw new Error(`operating brief prefetch failed: ${response.status}`)
    const brief = await response.json()
    if (brief?.contractVersion !== 'operating-brief/1') {
      throw new Error('operating brief prefetch returned an invalid contract')
    }
    sessionStorage.setItem(OPERATING_BRIEF_PREFETCH_KEY, JSON.stringify({
      cachedAt: new Date().toISOString(),
      brief,
    }))
  }).catch(() => undefined).finally(() => {
    operatingBriefPrefetch = null
  })
  return operatingBriefPrefetch
}

export interface OperatingAnalysisRevocationHistoryV1 {
  schema: 'OperatingAnalysisRevocationHistoryV1'
  availability: OperatingAnalysisAvailabilityV1['availability']
  recentWork?: {
    sessionId: string
    title: string
    updatedAt: string
  }
}

export async function getOperatingAnalysisAvailability(): Promise<OperatingAnalysisAvailabilityV1> {
  return (await get('/api/auth/operating-analysis-availability')) as unknown as OperatingAnalysisAvailabilityV1
}

export async function getOperatingAnalysisHistory(): Promise<OperatingAnalysisRevocationHistoryV1> {
  return (await get('/api/auth/operating-analysis-history')) as unknown as OperatingAnalysisRevocationHistoryV1
}

export async function exchangeOperatingAnalysis(): Promise<OperatingAnalysisExchangeV1> {
  return (await post('/api/auth/weknora-exchange')) as unknown as OperatingAnalysisExchangeV1
}

export async function createOperatingAnalysisHandoff(
  sourceSessionId: string,
  sourceMessageId: string,
): Promise<OperatingAnalysisHandoffV1> {
  return (await post('/api/v1/operating-analysis-handoffs', {
    sourceSessionId,
    sourceMessageId,
  })) as unknown as OperatingAnalysisHandoffV1
}

export async function consumeOperatingAnalysisHandoff(
  handoffRef: string,
): Promise<OperatingAnalysisHandoffExchangeV1> {
  return (await post(`/api/auth/operating-analysis-handoffs/${encodeURIComponent(handoffRef)}/consume`)) as unknown as OperatingAnalysisHandoffExchangeV1
}
