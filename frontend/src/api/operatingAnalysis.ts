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

export async function getOperatingAnalysisAvailability(): Promise<OperatingAnalysisAvailabilityV1> {
  return (await get('/api/auth/operating-analysis-availability')) as unknown as OperatingAnalysisAvailabilityV1
}

export async function exchangeOperatingAnalysis(): Promise<OperatingAnalysisExchangeV1> {
  return (await post('/api/auth/weknora-exchange')) as unknown as OperatingAnalysisExchangeV1
}
