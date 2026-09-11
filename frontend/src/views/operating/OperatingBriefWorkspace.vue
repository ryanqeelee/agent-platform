<template>
  <section class="operating-brief-workspace" aria-label="经营简报">
    <div v-if="loading" class="operating-brief-placeholder"><p>经营简报</p><h1>本周经营，先看这几件事</h1><p role="status">正在读取经营数据…</p></div>
    <p v-if="error" role="alert">{{ error }} <button @click="mountBrief">重试</button></p>
    <div ref="target" />
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY, exchangeOperatingAnalysis } from '@/api/operatingAnalysis'

import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const props = defineProps<{ active: boolean }>()
const router = useRouter()
const target = ref<HTMLElement | null>(null)
const error = ref('')
const loading = ref(true)
let mountGeneration = 0
let mounted = true
let controller: { setActive(active: boolean): void; dispose(): void } | null = null

async function mountBrief() {
  const generation = ++mountGeneration
  error.value = ''
  loading.value = true
  controller?.dispose()
  controller = null
  try {
    const response = await exchangeOperatingAnalysis()
    if (!mounted || generation !== mountGeneration) return
    if (!response.access_token || response.expires_in !== 900) throw new Error('无法获取经营数据访问权限。')
    localStorage.setItem('retail_ai_app_auth_token', response.access_token)
    document.cookie = `retail_ai_app_auth_token=${response.access_token}; Path=/app; Max-Age=900; SameSite=Lax`
    const entry = '/app/operating-brief.js'
    const { mountOperatingBrief } = await import(/* @vite-ignore */ entry)
    if (!mounted || generation !== mountGeneration || !target.value) return
    controller = mountOperatingBrief({
      target: target.value,
      onStartAnalysis(handoff: { schema: 'OperatingAnalysisHandoffV1'; question: string }) {
        sessionStorage.setItem(OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY, JSON.stringify(handoff))
        void router.push('/platform/operating-analysis')
      },
      onOpenAnalysis: () => { void router.push('/platform/operating-analysis') },
      onError: (reason: Error) => { error.value = reason.message },
    })
    controller?.setActive(props.active)
  } catch {
    if (!mounted || generation !== mountGeneration) return
    error.value = '经营简报加载失败，请重试。'
  } finally {
    if (mounted && generation === mountGeneration) loading.value = false
  }
}

watch([() => auth.user?.id, () => auth.selectedTenantId], () => {
  ++mountGeneration
  controller?.dispose()
  controller = null
  error.value = ''
  loading.value = true
  if (props.active) void mountBrief()
}, { immediate: true })
watch(() => props.active, active => {
  if (active && !controller) void mountBrief()
  else controller?.setActive(active)
})
onBeforeUnmount(() => { mounted = false; ++mountGeneration; controller?.dispose() })
</script>

<style scoped>
.operating-brief-workspace { flex: 1; min-width: 0; min-height: 0; overflow: auto; padding: 24px; }
.operating-brief-placeholder { padding: 24px; }
[role="alert"] { color: var(--td-error-color); }
</style>
