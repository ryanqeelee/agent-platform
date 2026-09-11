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

export async function prepareOperatingDataRead(): Promise<void> {
  const response = await exchangeOperatingAnalysis()
  if (!response.access_token || response.expires_in !== 900) throw new Error('无法获取经营数据访问权限。')
  localStorage.setItem('retail_ai_app_auth_token', response.access_token)
  document.cookie = `retail_ai_app_auth_token=${response.access_token}; Path=/app; Max-Age=900; SameSite=Lax`
}
