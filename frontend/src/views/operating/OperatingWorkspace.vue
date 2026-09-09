<template>
  <section
    class="operating-workspace"
    :class="{ 'has-reading-panel': active && surface === 'analysis' && readingVisible }"
    :aria-busy="active && (loadState === 'loading' || snapshot.auth === 'loading')"
    aria-label="经营工作区"
  >
    <div ref="runtimeRoot" class="operating-runtime-root" aria-hidden="true"></div>

    <div v-show="surface === 'analysis'" class="operating-analysis" :class="{ 'operating-analysis--home': isAnalysisHome }">
      <header v-show="!isAnalysisHome" class="operating-header">
        <div class="operating-header__identity">
          <span class="operating-header__eyebrow">经营分析</span>
          <h1>{{ snapshot.title || '新建分析' }}</h1>
        </div>
        <nav class="operating-header__nav" aria-label="经营工作区导航">
          <button type="button" class="operating-header__link" @click="goToSurface('brief')">
            经营简报
          </button>
          <button
            type="button"
            class="operating-header__link"
            :disabled="!controllerReady"
            @click="createSession"
          >
            新建分析
          </button>
        </nav>
      </header>

      <div ref="scrollContainer" class="operating-reading-column" @scroll="handleScroll">
        <div v-if="snapshot.sessionLoadError" class="operating-session-load-error" role="alert">
          <span class="operating-session-load-error__mark" aria-hidden="true">!</span>
          <h2>分析记录暂时无法显示</h2>
          <p>{{ snapshot.sessionLoadError.message }}</p>
          <button
            type="button"
            :disabled="snapshot.sessionLoadError.retrying"
            @click="retrySession"
          >
            {{ snapshot.sessionLoadError.retrying ? '正在重试…' : '重试加载' }}
          </button>
        </div>

        <div v-else-if="snapshot.messages.length === 0" class="operating-welcome">
          <p class="operating-welcome__kicker">经营分析</p>
          <h2>今天想先看哪项经营变化？</h2>
          <p>看销售、查毛利、找变化。可以直接提问，或上传经营数据文件。</p>
          <div ref="homeComposerTarget" class="operating-home-composer"></div>
          <div v-if="snapshot.attachments.length > 0 && !snapshot.uploadPending && !snapshot.draft.trim()" class="operating-examples" aria-label="文件分析起步问题">
            <button v-for="question in fileStarterQuestions" :key="question" type="button" @click="setDraft(question)">
              <span>{{ question }}</span>
              <span aria-hidden="true">↗</span>
            </button>
          </div>
        </div>

        <div v-else class="operating-messages" aria-live="polite">
          <article v-for="message in snapshot.messages" :key="message.id" class="operating-message">
            <usermsg v-if="message.role === 'user'" :content="message.text"
              :attachments="message.attachments?.map(file => ({ id: file.id, file_name: file.name }))" />
            <div v-else class="operating-assistant-message">
              <AgentStreamDisplay
                v-if="hasVisibleProcess(message)"
                :session="operatingProcessSession(message)"
                process-only
                :operating-status="message.status"
                :operating-message-id="message.id"
                @operating-result="inspectQuery"
              />
              <botmsg v-if="message.text" :content="message.text"
                :session="{ content: message.text, is_completed: message.status !== 'running' }" operating />
              <button
                v-if="message.report"
                type="button"
                class="operating-report-entry"
                @click="selectReport(message.report.artifactId)"
              >
                <t-icon name="file-1" />
                <span>查看本轮报告</span>
                <small>{{ message.report.title }}</small>
              </button>
            </div>
          </article>
        </div>

        <div ref="controlsTarget" class="operating-controls-portal" aria-live="polite"></div>
      </div>

      <Teleport :to="homeComposerTarget" :disabled="!isAnalysisHome || !homeComposerTarget">
        <div class="operating-composer-wrap">
          <div v-if="actionError" class="operating-action-error" role="alert">
            <span>{{ actionError }}</span>
            <button type="button" aria-label="关闭提示" @click="actionError = ''">×</button>
          </div>
          <InputField :operating="operatingComposer" />
          <p class="operating-composer-hint">经营分析支持 CSV 与 XLSX 数据文件</p>
        </div>
      </Teleport>
    </div>

    <div v-show="surface === 'brief'" class="operating-brief">
      <header class="operating-header operating-header--brief">
        <div class="operating-header__identity">
          <span class="operating-header__eyebrow">经营工作区</span>
          <h1>经营简报</h1>
        </div>
        <nav class="operating-header__nav" aria-label="经营工作区导航">
          <button type="button" class="operating-header__link" @click="goToSurface('analysis')">
            进入经营分析
          </button>
        </nav>
      </header>
      <div ref="briefTarget" class="operating-brief-portal"></div>
    </div>

    <ChatReadingPanel
      :visible="active && surface === 'analysis' && readingVisible"
      :title="readingTitle"
      close-label="关闭阅读"
      variant="reading"
      :overlay-breakpoint="1280"
      expandable
      keep-mounted
      :focus-on-open="readingFocusRequested"
      close-on-escape
      @close="closeReading"
    >
      <template v-if="readingCanReturn" #actions>
        <button type="button" class="operating-reading-back" @click="returnToReport">
          返回报告
        </button>
      </template>
      <div ref="reportTarget" class="operating-report-portal"></div>
    </ChatReadingPanel>

    <div v-if="active && loadIssue" class="operating-load-state" role="alert">
      <div class="operating-load-state__card">
        <span class="operating-load-state__mark" aria-hidden="true">!</span>
        <h2>经营工作区暂时无法打开</h2>
        <p>{{ loadIssue }}</p>
        <div class="operating-load-state__actions">
          <button type="button" @click="retryRuntime">重新加载</button>
          <button v-if="snapshot.auth === 'error'" type="button" class="is-secondary" @click="reauthorize">
            重新授权
          </button>
          <button type="button" class="is-secondary" @click="router.push('/platform/creatChat')">
            返回员工助理
          </button>
        </div>
      </div>
    </div>
    <div v-else-if="active && (loadState === 'loading' || snapshot.auth === 'loading')" class="operating-load-state" role="status">
      <span class="operating-loader" aria-hidden="true"></span>
      <p>正在进入经营工作区…</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import InputField from '@/components/Input-field.vue'
import ChatReadingPanel from '@/components/ChatReadingPanel.vue'
import AgentStreamDisplay from '@/views/chat/components/AgentStreamDisplay.vue'
import usermsg from '@/views/chat/components/usermsg.vue'
import botmsg from '@/views/chat/components/botmsg.vue'
import {
  loadOperatingClient,
  type OperatingController,
  type OperatingLocation,
  type OperatingMessage,
  type OperatingSnapshot,
} from './operatingClient'
import {
  isOperatingRoutePath,
  operatingLocationFromRoute,
  operatingLocationKey,
  operatingNavigationStateFromRoute,
  operatingRouteLocationFromRuntime,
  runtimeNavigationTarget,
  shouldRestoreOperatingLocation,
} from './operatingHost'
import { operatingProcessSession } from './operatingPresentation'

const emit = defineEmits<{
  (event: 'controller-change', controller: OperatingController | null): void
}>()

const APP_AUTH_TOKEN_KEY = 'retail_ai_app_auth_token'
const HANDOFF_PROMPT_KEY = 'operating_analysis_handoff_prompt_v1'

const fileStarterQuestions = [
  '请检查上传文件的字段、时间范围和缺失情况，说明可以支持哪些经营分析。',
  '请概括上传文件中的主要信息，指出值得进一步核查的变化，并说明数据限制。',
]

const route = useRoute()
const router = useRouter()
const runtimeRoot = ref<HTMLElement | null>(null)
const reportTarget = ref<HTMLElement | null>(null)
const controlsTarget = ref<HTMLElement | null>(null)
const briefTarget = ref<HTMLElement | null>(null)
const scrollContainer = ref<HTMLElement | null>(null)
const homeComposerTarget = ref<HTMLElement | null>(null)
const loadState = ref<'loading' | 'ready' | 'error'>('loading')
const loadError = ref('')
const actionError = ref('')
const active = computed(() => isOperatingRoutePath(route.path))
const surface = computed(() => operatingLocationFromRoute(route.path, route.query).surface)
const isAnalysisHome = computed(() => !snapshot.value.sessionLoadError && snapshot.value.messages.length === 0)
const titleBeforeHost = document.title

function emptySnapshot(): OperatingSnapshot {
  return {
    auth: 'loading',
    error: null,
    title: '新建分析',
    location: operatingLocationFromRoute(route.path, route.query),
    draft: '',
    running: false,
    disabled: true,
    cancelling: false,
    sessionsLoading: true,
    sessions: [],
    sessionLoadError: null,
    messages: [],
    attachments: [],
    uploadAvailable: false,
    uploadPending: false,
    artifacts: [],
    selectedArtifactId: null,
    reading: { kind: 'following-current' },
  }
}

const snapshot = shallowRef<OperatingSnapshot>(emptySnapshot())
const controllerReady = computed(() => loadState.value === 'ready' && snapshot.value.auth === 'ready')
const loadIssue = computed(() => loadError.value || (snapshot.value.auth === 'error'
  ? snapshot.value.error || '经营分析授权已失效，请重新授权。'
  : ''))

let controller: OperatingController | null = null
let unsubscribe: (() => void) | null = null
let runtimeGeneration = 0
let pendingRuntimeRouteKey: string | null = null
let stickToBottom = true
let wasActive = false

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

function publishSnapshot() {
  if (!controller) return
  try {
    snapshot.value = controller.getSnapshot()
  } catch (error) {
    loadState.value = 'error'
    loadError.value = errorMessage(error, '经营分析状态读取失败，请重试。')
  }
}

function handleRuntimeNavigate(location: OperatingLocation, mode: 'push' | 'replace') {
  const target = runtimeNavigationTarget(active.value, route.path, route.query, location)
  if (!target) return
  pendingRuntimeRouteKey = operatingLocationKey(location)
  void router[mode](target).catch((error) => {
    pendingRuntimeRouteKey = null
    actionError.value = errorMessage(error, '无法切换经营分析页面。')
  })
}

function disposeRuntime() {
  runtimeGeneration += 1
  unsubscribe?.()
  unsubscribe = null
  emit('controller-change', null)
  controller?.dispose()
  controller = null
}

async function startRuntime() {
  const generation = ++runtimeGeneration
  loadState.value = 'loading'
  loadError.value = ''
  actionError.value = ''
  await nextTick()

  let nextController: OperatingController | null = null
  try {
    if (!runtimeRoot.value || !reportTarget.value || !controlsTarget.value || !briefTarget.value) {
      throw new Error('经营分析挂载节点尚未就绪，请重试。')
    }
    const runtime = await loadOperatingClient()
    if (generation !== runtimeGeneration) return
    nextController = runtime.createController({
      runtimeRoot: runtimeRoot.value,
      initialLocation: operatingLocationFromRoute(route.path, route.query),
      onNavigate: handleRuntimeNavigate,
    })
    if (generation !== runtimeGeneration) {
      nextController.dispose()
      return
    }

    controller = nextController
    unsubscribe = controller.subscribe(publishSnapshot)
    publishSnapshot()
    controller.mountReport(reportTarget.value)
    controller.mountControls(controlsTarget.value)
    controller.mountBrief(briefTarget.value)
    controller.setActive(active.value)
    loadState.value = 'ready'
    emit('controller-change', controller)
  } catch (error) {
    nextController?.dispose()
    if (generation !== runtimeGeneration) return
    emit('controller-change', null)
    controller = null
    unsubscribe = null
    loadState.value = 'error'
    loadError.value = errorMessage(error, '经营分析组件加载失败，请重试。')
  }
}

function retryRuntime() {
  disposeRuntime()
  snapshot.value = emptySnapshot()
  void startRuntime()
}

function reauthorize() {
  window.location.assign(router.resolve({ path: route.path, query: route.query }).href)
}

function handleStorage(event: StorageEvent) {
  if (active.value && event.storageArea === localStorage
    && event.key === APP_AUTH_TOKEN_KEY && event.newValue === null) {
    reauthorize()
  }
}

function syncRouteToRuntime() {
  if (!active.value) {
    pendingRuntimeRouteKey = null
    controller?.setActive(false)
    document.title = titleBeforeHost
    wasActive = false
    return
  }

  const location = operatingLocationFromRoute(route.path, route.query)
  const firstActivation = !wasActive
  wasActive = true
  document.title = `环枢｜${surface.value === 'brief' ? '经营简报' : snapshot.value.title || '经营分析'}`
  if (!controller) return

  // A handoff explicitly starts a new analysis. Recreate the existing runtime
  // once so its established handoff reader consumes the prompt; normal visits
  // retain the mounted controller, stream and draft.
  if (sessionStorage.getItem(HANDOFF_PROMPT_KEY) !== null) {
    retryRuntime()
    return
  }
  const routeState = operatingNavigationStateFromRoute(route.path, route.query)
  const previous = snapshot.value.location
  if (shouldRestoreOperatingLocation(
    firstActivation,
    sessionStorage.getItem(HANDOFF_PROMPT_KEY) !== null,
    routeState,
    previous,
  )) {
    const restored = { ...previous, surface: location.surface }
    const target = operatingRouteLocationFromRuntime(restored)
    if (target) {
      controller.navigate(restored)
      controller.setActive(true)
      pendingRuntimeRouteKey = operatingLocationKey(restored)
      void router.replace(target).catch((error) => {
        pendingRuntimeRouteKey = null
        actionError.value = errorMessage(error, '无法恢复上次经营分析。')
      })
      return
    }
  }

  const key = operatingLocationKey(location)
  if (pendingRuntimeRouteKey === key) {
    pendingRuntimeRouteKey = null
  } else {
    pendingRuntimeRouteKey = null
    controller.navigate(location)
  }
  controller.setActive(true)
}

function goToSurface(nextSurface: OperatingLocation['surface']) {
  const target = operatingRouteLocationFromRuntime({
    ...operatingLocationFromRoute(route.path, route.query),
    surface: nextSurface,
  })
  if (target) void router.push(target)
}

function setDraft(text: string) {
  try {
    controller?.setDraft(text)
  } catch (error) {
    actionError.value = errorMessage(error, '暂时无法编辑问题。')
  }
}

function selectReport(id: string) {
  controller?.selectArtifact(id)
}

function inspectQuery(messageId: string, queryId: string) {
  controller?.inspectQuery(messageId, queryId)
}

function closeReading() {
  controller?.closeReading()
}

function returnToReport() {
  controller?.returnToReport()
}

const readingVisible = computed(() => {
  if (snapshot.value.sessionLoadError) return false
  const reading = snapshot.value.reading
  if (reading.kind === 'closed') return false
  if (reading.kind === 'following-current') return snapshot.value.selectedArtifactId !== null
  return true
})
const readingTitle = computed(() => {
  const reading = snapshot.value.reading
  if (reading.kind === 'inspect-query') return reading.target.title
  if (reading.kind === 'inspect' && reading.target.kind === 'query') return reading.target.title
  const selected = snapshot.value.artifacts.find((artifact) => artifact.id === snapshot.value.selectedArtifactId)
  return selected?.title || '经营分析报告'
})
const readingCanReturn = computed(() => {
  const reading = snapshot.value.reading
  return reading.kind === 'inspect' && reading.target.kind === 'query' && reading.target.artifactId !== null
})
const readingFocusRequested = computed(() => {
  const reading = snapshot.value.reading
  return reading.kind === 'inspect' || reading.kind === 'inspect-query'
})

const operatingComposer = computed(() => ({
  draft: snapshot.value.draft,
  disabled: !controllerReady.value || snapshot.value.disabled,
  running: snapshot.value.running,
  cancelling: snapshot.value.cancelling,
  uploadPending: snapshot.value.uploadPending,
  uploadAvailable: snapshot.value.uploadAvailable,
  attachments: snapshot.value.attachments.map((attachment) => ({ ...attachment })),
  setDraft,
  send(text: string) {
    try {
      return controller?.send(text) ?? false
    } catch (error) {
      actionError.value = errorMessage(error, '经营分析暂时无法发送，请重试。')
      return false
    }
  },
  stop() {
    try {
      controller?.cancel()
    } catch (error) {
      actionError.value = errorMessage(error, '暂时无法停止分析。')
    }
  },
  async upload(files: File[]) {
    if (!controller) return
    try {
      await controller.upload(files)
    } catch (error) {
      actionError.value = errorMessage(error, '数据文件上传失败，请重试。')
      throw error
    }
  },
  async remove(id: string) {
    if (!controller) return
    try {
      await controller.removeAttachment(id)
    } catch (error) {
      actionError.value = errorMessage(error, '暂时无法移除数据文件。')
      throw error
    }
  },
}))

function hasVisibleProcess(message: OperatingMessage): boolean {
  return message.status === 'running'
    || message.process.planItems.length > 0
    || message.process.queries.length > 0
    || message.process.calculations.length > 0
}

function createSession() {
  try {
    controller?.newSession()
  } catch (error) {
    actionError.value = errorMessage(error, '暂时无法新建分析。')
  }
}

async function retrySession() {
  if (!controller || snapshot.value.sessionLoadError?.retrying) return
  actionError.value = ''
  try {
    await controller.retrySession()
  } catch (error) {
    actionError.value = errorMessage(error, '分析记录重试失败，请稍后再试。')
  }
}

function handleScroll() {
  const element = scrollContainer.value
  if (!element) return
  stickToBottom = element.scrollHeight - element.scrollTop - element.clientHeight < 96
}

watch(() => route.fullPath, syncRouteToRuntime, { immediate: true })
watch(() => snapshot.value.title, () => {
  if (active.value && surface.value === 'analysis') {
    document.title = `环枢｜${snapshot.value.title || '经营分析'}`
  }
})
watch(() => snapshot.value.messages, () => {
  if (!stickToBottom || (!snapshot.value.running && snapshot.value.selectedArtifactId)) return
  nextTick(() => {
    const element = scrollContainer.value
    if (element) element.scrollTop = element.scrollHeight
  })
}, { deep: true })

onMounted(() => {
  window.addEventListener('storage', handleStorage)
  void startRuntime()
})

onBeforeUnmount(() => {
  window.removeEventListener('storage', handleStorage)
  disposeRuntime()
  document.title = titleBeforeHost
})
</script>

<style scoped lang="less">
.operating-workspace {
  position: relative;
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-container);
}

.operating-runtime-root { display: none; }

.operating-analysis,
.operating-brief {
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
  flex-direction: column;
}

.operating-header {
  position: relative;
  z-index: 5;
  display: flex;
  min-height: 68px;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 10px clamp(20px, 4vw, 56px);
  border-bottom: 1px solid var(--td-component-stroke);
  background: color-mix(in srgb, var(--td-bg-color-container) 94%, transparent);
  backdrop-filter: blur(12px);
}

.operating-header__identity {
  min-width: 0;

  h1 {
    margin: 2px 0 0;
    overflow: hidden;
    color: var(--td-text-color-primary);
    font-size: 18px;
    font-weight: 650;
    line-height: 1.35;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.operating-header__eyebrow {
  color: var(--td-brand-color);
  font-size: 11px;
  font-weight: 650;
  letter-spacing: .08em;
}

.operating-header__nav {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: 6px;
}

.operating-header__link {
  display: inline-flex;
  min-height: 36px;
  align-items: center;
  gap: 6px;
  padding: 0 11px;
  border: 0;
  border-radius: 8px;
  color: var(--td-text-color-secondary);
  background: transparent;
  font: inherit;
  font-size: 13px;
  white-space: nowrap;
  cursor: pointer;

  &:hover,
  &:focus-visible {
    color: var(--td-brand-color);
    background: var(--td-brand-color-light);
    outline: none;
  }

  &:disabled {
    cursor: not-allowed;
    opacity: .45;
  }
}

.operating-reading-column {
  min-height: 0;
  flex: 1;
  overflow: auto;
  padding: clamp(28px, 5vh, 64px) clamp(20px, 5vw, 72px) 36px;
  scroll-behavior: smooth;
}

@media (min-width: 1280px) {
  .operating-workspace.has-reading-panel {
    --chat-reading-width: min(760px, 50vw);
    padding-right: var(--chat-reading-width);
    box-sizing: border-box;

    .operating-header {
      flex-wrap: wrap;
      padding-inline: 20px;
      gap: 4px;
    }

    .operating-header__identity,
    .operating-header__nav { width: 100%; }

    .operating-reading-column { padding-inline: 20px; }
    .operating-composer-wrap { padding-inline: 20px; }
  }
}

.operating-welcome,
.operating-session-load-error,
.operating-messages,
.operating-controls-portal,
.operating-composer-wrap {
  width: min(100%, 1080px);
  margin-inline: auto;
}

.operating-session-load-error {
  box-sizing: border-box;
  display: grid;
  max-width: 560px;
  justify-items: start;
  gap: 10px;
  margin-top: clamp(36px, 10vh, 96px);
  padding: 24px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
  background: color-mix(in srgb, var(--td-warning-color-1) 42%, var(--td-bg-color-container));

  h2,
  p { margin: 0; }

  h2 { font-size: 18px; }

  p {
    color: var(--td-text-color-secondary);
    line-height: 1.6;
  }

  button {
    min-height: 36px;
    padding: 0 14px;
    border: 0;
    border-radius: 8px;
    color: var(--td-text-color-anti);
    background: var(--td-brand-color);
    font: inherit;
    cursor: pointer;

    &:disabled {
      cursor: wait;
      opacity: .6;
    }
  }
}

.operating-session-load-error__mark {
  display: grid;
  width: 28px;
  height: 28px;
  place-items: center;
  border-radius: 50%;
  color: var(--td-warning-color);
  background: var(--td-warning-color-1);
  font-weight: 700;
}

.operating-welcome {
  display: flex;
  min-height: min(240px, 30vh);
  flex-direction: column;
  justify-content: center;
  padding-bottom: 24px;

  h2 {
    max-width: 720px;
    margin: 6px 0 12px;
    font-size: clamp(28px, 4vw, 44px);
    font-weight: 620;
    letter-spacing: -.035em;
    line-height: 1.18;
  }

  > p:not(.operating-welcome__kicker) {
    margin: 0;
    color: var(--td-text-color-secondary);
    font-size: 15px;
  }
}

.operating-welcome__kicker {
  margin: 0;
  color: var(--td-brand-color);
  font-size: 12px;
  font-weight: 650;
  letter-spacing: .08em;
}

.operating-examples {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 10px;
  margin-top: 30px;

  button {
    display: flex;
    min-height: 92px;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    padding: 16px;
    border: 1px solid var(--td-component-stroke);
    border-radius: 12px;
    color: var(--td-text-color-primary);
    background: color-mix(in srgb, var(--td-bg-color-container) 88%, var(--td-brand-color-light));
    font: inherit;
    font-size: 14px;
    line-height: 1.55;
    text-align: left;
    cursor: pointer;
    transition: border-color .16s ease, transform .16s ease, box-shadow .16s ease;

    &:hover,
    &:focus-visible {
      border-color: color-mix(in srgb, var(--td-brand-color) 45%, var(--td-component-stroke));
      outline: none;
      box-shadow: 0 8px 24px color-mix(in srgb, var(--td-brand-color) 10%, transparent);
      transform: translateY(-1px);
    }

    span:last-child { color: var(--td-brand-color); }
  }
}

.operating-messages {
  display: flex;
  flex-direction: column;
  gap: 28px;
}

.operating-message,
.operating-assistant-message { min-width: 0; }

.operating-assistant-message { width: min(100%, 920px); }

.operating-assistant-message__text {
  margin: 10px 0 0;
  padding-left: 26px;
  color: var(--td-text-color-primary);
  font-size: 15px;
  line-height: 1.7;
  white-space: pre-wrap;
}

.operating-controls-portal {
  min-height: 1px;
  margin-top: 24px;
}

.operating-report-entry {
  display: grid;
  width: min(calc(100% - 26px), 520px);
  box-sizing: border-box;
  min-height: 48px;
  grid-template-columns: auto auto minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  margin: 12px 0 0 26px;
  padding: 9px 12px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-container);
  font: inherit;
  text-align: left;
  cursor: pointer;

  &:hover,
  &:focus-visible {
    border-color: color-mix(in srgb, var(--td-brand-color) 50%, var(--td-component-stroke));
    outline: none;
    background: var(--td-brand-color-light);
  }

  > :deep(.t-icon) { color: var(--td-brand-color); }
  > span { font-size: 13px; font-weight: 600; }
  > small {
    min-width: 0;
    overflow: hidden;
    color: var(--td-text-color-secondary);
    font-size: 12px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
}

.operating-reading-back {
  min-height: 32px;
  padding: 0 10px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container);
  font: inherit;
  font-size: 12px;
  cursor: pointer;

  &:hover,
  &:focus-visible { color: var(--td-brand-color); border-color: var(--td-brand-color); outline: none; }
}

.operating-report-portal {
  width: 100%;
  min-width: 0;
  min-height: 1px;
  overflow-x: clip;
}

.operating-report-portal :deep(.operating-result-fragment) { min-width: 0; }
.operating-report-portal :deep(.ant-table-wrapper) { max-width: 100%; overflow-x: auto; }

.operating-composer-wrap {
  width: min(100%, 1224px);
  flex: 0 0 auto;
  padding: 12px clamp(20px, 5vw, 72px) 14px;
  box-sizing: border-box;
  background: linear-gradient(180deg, transparent, var(--td-bg-color-container) 22%);
}

.operating-composer-hint {
  margin: 6px 4px 0;
  color: var(--td-text-color-placeholder);
  font-size: 11px;
  text-align: center;
}

.operating-action-error {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
  padding: 8px 12px;
  border-radius: 8px;
  color: var(--td-error-color);
  background: var(--td-error-color-light);
  font-size: 12px;

  button { border: 0; color: inherit; background: transparent; cursor: pointer; }
}

.operating-brief-portal {
  min-width: 0;
  min-height: 0;
  flex: 1;
  overflow: auto;
}

.operating-load-state {
  position: absolute;
  z-index: 20;
  inset: 0;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 14px;
  padding: 24px;
  color: var(--td-text-color-secondary);
  background: var(--td-bg-color-container);
}

.operating-loader {
  width: 28px;
  height: 28px;
  border: 2px solid var(--td-component-stroke);
  border-top-color: var(--td-brand-color);
  border-radius: 50%;
  animation: operating-spin .8s linear infinite;
}

.operating-load-state__card {
  width: min(100%, 460px);
  padding: 32px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 16px;
  background: var(--td-bg-color-container);
  box-shadow: var(--td-shadow-2);
  text-align: center;

  h2 { margin: 14px 0 8px; color: var(--td-text-color-primary); font-size: 20px; }
  p { margin: 0; line-height: 1.6; }
}

.operating-load-state__mark {
  display: inline-grid;
  width: 38px;
  height: 38px;
  place-items: center;
  border-radius: 50%;
  color: var(--td-error-color);
  background: var(--td-error-color-light);
  font-size: 20px;
  font-weight: 700;
}

.operating-load-state__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 8px;
  margin-top: 22px;

  button {
    min-height: 36px;
    padding: 0 14px;
    border: 1px solid var(--td-brand-color);
    border-radius: 8px;
    color: var(--td-text-color-anti);
    background: var(--td-brand-color);
    font: inherit;
    cursor: pointer;

    &.is-secondary {
      color: var(--td-text-color-primary);
      border-color: var(--td-component-stroke);
      background: var(--td-bg-color-container);
    }
  }
}

@keyframes operating-spin { to { transform: rotate(360deg); } }

@media (max-width: 760px) {
  .operating-header { min-height: 58px; padding-inline: 14px; flex-wrap: wrap; gap: 4px; }
  .operating-header__identity { width: 100%; }
  .operating-header__identity h1 { font-size: 15px; }
  .operating-header__eyebrow { display: none; }
  .operating-header__nav { width: 100%; }
  .operating-header__link { padding-inline: 8px; }
  .operating-reading-column { padding: 24px 14px; }
  .operating-examples { grid-template-columns: 1fr; }
  .operating-examples button { min-height: 68px; }
  .operating-composer-wrap { padding: 10px 14px 12px; box-sizing: border-box; }
}

.operating-analysis--home {
  .operating-reading-column { padding: clamp(36px, 12vh, 100px) 28px 40px; }
  .operating-welcome, .operating-controls-portal { max-width: 760px; }
  .operating-welcome { min-height: 0; padding-bottom: 24px; }
  .operating-welcome__kicker { margin-bottom: 14px; font-weight: 600; letter-spacing: .06em; }
  .operating-welcome h2 { margin: 0; font-size: clamp(28px, 3.2vw, 40px); }
  .operating-welcome > p:not(.operating-welcome__kicker) { margin-top: 16px; line-height: 1.8; }
  .operating-home-composer { margin-top: 28px; }
  .operating-composer-wrap { width: 100%; padding: 0; background: none; }
  .operating-composer-wrap :deep(.answers-input) { width: 100%; max-width: 100%; margin: 0; }
  .operating-composer-wrap :deep(.rich-input-container) { max-width: none; border-radius: 16px; }
  .operating-composer-wrap :deep(.t-textarea__inner) { min-height: 96px; font-size: 15px; line-height: 1.7; }
  .operating-examples { grid-template-columns: repeat(2, minmax(0, 1fr)); margin-top: 24px; }
}

@media (max-width: 760px) {
  .operating-analysis--home .operating-reading-column { padding: 32px 18px; }
  .operating-analysis--home .operating-welcome h2 { font-size: 28px; }
  .operating-analysis--home .operating-examples { grid-template-columns: 1fr; }
}

@media (prefers-reduced-motion: reduce) {
  .operating-reading-column { scroll-behavior: auto; }
  .operating-examples button { transition: none; }
  .operating-loader { animation-duration: 1.6s; }
}
</style>
