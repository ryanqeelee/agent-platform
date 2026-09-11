<template>
  <section class="operating-analysis-home" aria-label="经营分析">
    <div class="operating-welcome">
      <p class="operating-welcome__kicker">经营分析</p>
      <h1>今天想先看哪项经营变化？</h1>
      <p>看销售、查毛利、找变化。可以直接提问，或上传经营数据文件。</p>
      <div class="operating-home-composer">
        <InputField
          ref="inputFieldRef"
          :agent-id="BUILTIN_OPERATING_ANALYST_ID"
          @send-msg="(query, modelId, mentionedItems, imageFiles, attachmentFiles, webSearchEnabled) => createAnalysis(query, modelId, mentionedItems, imageFiles, attachmentFiles, webSearchEnabled)"
        />
        <p class="operating-composer-hint">可上传图片、表格或文档，结合经营数据分析</p>
      </div>
      <p v-if="errorMessage" class="operating-home-error" role="alert">{{ errorMessage }}</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import InputField from '@/components/Input-field.vue'
import { BUILTIN_OPERATING_ANALYST_ID } from '@/api/agent'
import { createSessions } from '@/api/chat'
import { OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY } from '@/api/operatingAnalysis'
import { useMenuStore } from '@/stores/menu'
import { useAuthStore } from '@/stores/auth'
import { ownedOperatingPromptQuestion, sameOperatingPromptOwner } from './operatingNavigation'

const router = useRouter()
const auth = useAuthStore()
const promptOwner = () => ({ actorId: String(auth.user?.id ?? ''), tenantId: String(auth.effectiveTenantId ?? '') })
const menuStore = useMenuStore()
const inputFieldRef = ref<InstanceType<typeof InputField> | null>(null)
const errorMessage = ref('')
let creating = false

async function createAnalysis(
  value: string,
  modelId = '',
  mentionedItems: unknown[] = [],
  imageFiles: unknown[] = [],
  attachmentFiles: unknown[] = [],
  webSearchEnabled = false,
) {
  if (creating) return
  const owner = promptOwner()
  creating = true
  errorMessage.value = ''
  try {
    const response = await createSessions({}) as any
    if (!sameOperatingPromptOwner(owner, promptOwner())) return
    const sessionId = String(response?.data?.id || '')
    if (!sessionId) throw new Error('missing session')
    menuStore.rememberOperatingSession(sessionId)
    const now = new Date().toISOString()
    menuStore.updataMenuChildren({
      title: '新建分析',
      path: `operating-analysis/chat/${sessionId}`,
      id: sessionId,
      isMore: false,
      isNoTitle: true,
      created_at: now,
      updated_at: now,
      last_request_state: { agent_id: BUILTIN_OPERATING_ANALYST_ID },
    })
    menuStore.changeIsFirstSession(true)
    menuStore.changeFirstQuery(value, mentionedItems, modelId, imageFiles, attachmentFiles, webSearchEnabled)
    await router.push(`/platform/operating-analysis/chat/${sessionId}`)
  } catch {
    errorMessage.value = '经营分析暂时无法开始，请稍后重试。'
  } finally {
    creating = false
  }
}

onMounted(() => {
  const raw = sessionStorage.getItem(OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY)
  if (!raw) return
  sessionStorage.removeItem(OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY)
  const question = ownedOperatingPromptQuestion(raw, promptOwner())
  if (question) void createAnalysis(question)
})
</script>

<style scoped lang="less">
.operating-analysis-home {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow: auto;
  padding: clamp(36px, 12vh, 100px) 28px 40px;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-container);
}

.operating-welcome {
  display: flex;
  width: min(100%, 760px);
  min-height: 0;
  margin-inline: auto;
  flex-direction: column;
  justify-content: center;

  h1 {
    max-width: 720px;
    margin: 0;
    font-size: clamp(28px, 3.2vw, 40px);
    font-weight: 620;
    letter-spacing: -.035em;
    line-height: 1.18;
  }

  > p:not(.operating-welcome__kicker):not(.operating-home-error) {
    margin: 16px 0 0;
    color: var(--td-text-color-secondary);
    font-size: 15px;
    line-height: 1.8;
  }
}

.operating-welcome__kicker {
  margin: 0 0 14px;
  color: var(--td-brand-color);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: .06em;
}

.operating-home-composer {
  margin-top: 28px;

  :deep(.answers-input) {
    position: static;
    width: 100%;
    max-width: 100%;
    transform: none;
  }

  :deep(.rich-input-container) {
    max-width: none;
    border-radius: 16px;
  }

  :deep(.t-textarea__inner) {
    min-height: 96px !important;
    font-size: 15px;
    line-height: 1.7;
  }
}

.operating-composer-hint {
  margin: 6px 4px 0;
  color: var(--td-text-color-placeholder);
  font-size: 11px;
  text-align: center;
}

.operating-home-error {
  margin: 12px 0 0;
  color: var(--td-error-color);
  font-size: 13px;
}

@media (max-width: 760px) {
  .operating-analysis-home { padding: 32px 18px; }
  .operating-welcome h1 { font-size: 28px; }
}
</style>
