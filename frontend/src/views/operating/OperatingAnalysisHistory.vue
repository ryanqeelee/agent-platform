<template>
  <section class="operating-history" aria-label="历史经营分析">
    <header><h2>历史经营分析</h2><RouterLink to="/platform/operating-analysis">新建分析</RouterLink></header>
    <p>迁移前的记录仅供查看，新分析使用经营分析智能体。</p>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-else-if="loading">正在读取记录…</p>
    <template v-else-if="sessionId">
      <article v-for="message in messages" :key="message.message_id">
        <usermsg v-if="message.role === 'user'" :content="message.content" />
        <botmsg v-else :content="message.content" :session="{ id: message.message_id, is_completed: true }" operating />
      </article>
      <p v-if="!messages.length">暂无已保存的消息。</p>
    </template>
    <ul v-else>
      <li v-for="session in sessions" :key="session.session_id">
        <RouterLink :to="`/platform/operating-analysis/history/${encodeURIComponent(session.session_id)}`">{{ session.title || '未命名分析' }}</RouterLink>
      </li>
    </ul>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import usermsg from '@/views/chat/components/usermsg.vue'
import botmsg from '@/views/chat/components/botmsg.vue'

const route = useRoute()
const sessionId = computed(() => typeof route.params.sessionId === 'string' ? route.params.sessionId : '')
const sessions = ref<Array<{ session_id: string; title: string }>>([])
const messages = ref<Array<{ message_id: string; role: string; content: string }>>([])
const loading = ref(false)
const error = ref('')

watch(sessionId, async (id, _, onCleanup) => {
  const controller = new AbortController()
  onCleanup(() => controller.abort())
  loading.value = true
  error.value = ''
  try {
    const headers: Record<string, string> = {
      Authorization: `Bearer ${localStorage.getItem('retail_ai_app_auth_token') || ''}`,
    }
    const platformAuthorization = localStorage.getItem('weknora_token')?.trim()
    const platformTenantId = localStorage.getItem('weknora_selected_tenant_id')?.trim()
    if (platformAuthorization) headers['x-platform-authorization'] = `Bearer ${platformAuthorization}`
    if (platformTenantId) headers['x-platform-tenant-id'] = platformTenantId
    const response = await fetch(`/api/agents/data/sessions${id ? `/${encodeURIComponent(id)}/messages` : ''}`, {
      headers,
      signal: controller.signal,
    })
    if (!response.ok) throw new Error('history unavailable')
    const value = await response.json()
    if (controller.signal.aborted) return
    if (id) messages.value = value
    else sessions.value = value
  } catch {
    if (!controller.signal.aborted) error.value = '历史分析暂时无法读取，请重新进入后重试。'
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}, { immediate: true })
</script>

<style scoped>
.operating-history { flex: 1; min-height: 0; overflow: auto; padding: 24px; }
header { display: flex; align-items: center; justify-content: space-between; }
article { margin: 24px auto; max-width: 960px; }
li { margin: 12px 0; }
</style>
