import { get } from '@/utils/request'

export type EnterpriseAdministrationTarget = 'members' | 'knowledge' | 'audit' | 'service_health'
export type EnterpriseAdministrationPriority = 'critical' | 'high' | 'medium'
export type EnterpriseAdministrationItemCode =
  | 'license_or_capability_attention'
  | 'edge_node_offline'
  | 'operating_analysis_access_gap'
  | 'pending_invitations'
  | 'knowledge_processing_failed'
  | 'recent_high_risk_operations'

export interface EnterpriseAdministrationQueueV1 {
  contract_version: 'EnterpriseAdministrationQueueV1'
  as_of: string
  summary: {
    service_level: string
    status: string
    member_quota?: number
    health: string
    member_usage: number
    storage_usage_bytes: number
    storage_quota_bytes: number
  }
  items: Array<{
    code: EnterpriseAdministrationItemCode
    priority: EnterpriseAdministrationPriority
    count: number
    target: EnterpriseAdministrationTarget
  }>
}

export async function getEnterpriseAdministrationQueue(): Promise<EnterpriseAdministrationQueueV1> {
  return (await get('/api/v1/enterprise-administration/queue')) as unknown as EnterpriseAdministrationQueueV1
}
