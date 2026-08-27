import { computed, onMounted, ref, watch } from 'vue'
import { useUIStore } from '@/stores/ui'
import { useChatResourcesStore } from '@/stores/chatResources'
import { useAuthStore } from '@/stores/auth'
import {
  evaluateTenantModelReadiness,
  type TenantModelReadiness,
} from '@/utils/tenantModelReadiness'

export function useTenantModelReadiness() {
  const uiStore = useUIStore()
  const chatResources = useChatResourcesStore()
  const authStore = useAuthStore()
  const readiness = ref<TenantModelReadiness | null>(null)
  const loaded = ref(false)
  const loading = ref(false)

  const refresh = async (force = false) => {
    if (!authStore.isSystemAdmin) {
      loaded.value = true
      return
    }
    loading.value = true
    try {
      await chatResources.ensureModels(force)
      readiness.value = evaluateTenantModelReadiness(chatResources.allModels)
    } finally {
      loading.value = false
      loaded.value = true
    }
  }

  onMounted(() => {
    refresh()
  })

  watch(
    () => uiStore.showSettingsModal,
    (open, wasOpen) => {
      if (authStore.isSystemAdmin && wasOpen && !open) {
        refresh(true)
      }
    },
  )

  const isReadyForDocumentKb = computed(
    () => !authStore.isSystemAdmin || (readiness.value?.isReadyForDocumentKb ?? false),
  )

  const isReadyForAgent = computed(
    () => !authStore.isSystemAdmin || (readiness.value?.isReadyForAgent ?? false),
  )

  const hasChat = computed(
    () => !authStore.isSystemAdmin || (readiness.value?.hasChat ?? false),
  )

  return {
    readiness,
    loaded,
    loading,
    refresh,
    isReadyForDocumentKb,
    isReadyForAgent,
    hasChat,
  }
}
