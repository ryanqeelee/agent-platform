export function platformTenantPath(tenantId: number, resource: string): string {
  if (!Number.isSafeInteger(tenantId) || tenantId <= 0) throw new Error('请先选择企业')
  return `/api/v1/system/admin/tenants/${tenantId}/${resource.replace(/^\/+/, '')}`
}
