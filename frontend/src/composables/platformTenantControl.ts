import { inject, readonly, ref, type InjectionKey, type Ref } from 'vue'

export const platformTenantControlIDKey: InjectionKey<Readonly<Ref<number | undefined>>> =
  Symbol('platform-tenant-control-id')

const noTenantControlID = readonly(ref<number>())

export function usePlatformTenantControlID(): Readonly<Ref<number | undefined>> {
  return inject(platformTenantControlIDKey, noTenantControlID)
}
