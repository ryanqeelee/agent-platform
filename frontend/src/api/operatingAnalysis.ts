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

export interface OperatingAnalysisHandoffV1 {
  schema: 'OperatingAnalysisHandoffV1'
  handoffRef: string
  expiresAt: string
}

export interface OperatingAnalysisHandoffConsumedV1 {
  schema: 'OperatingAnalysisHandoffV1'
  question: string
}

export const OPERATING_ANALYSIS_HANDOFF_REF_KEY = 'operating_analysis_handoff_ref_v1'
export const OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY = 'operating_analysis_handoff_prompt_v1'
export async function getOperatingAnalysisAvailability(): Promise<OperatingAnalysisAvailabilityV1> {
  return (await get('/api/v1/operating-analysis-availability')) as unknown as OperatingAnalysisAvailabilityV1
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
): Promise<OperatingAnalysisHandoffConsumedV1> {
  return (await post(`/api/v1/operating-analysis-handoffs/${encodeURIComponent(handoffRef)}/consume`)) as unknown as OperatingAnalysisHandoffConsumedV1
}
