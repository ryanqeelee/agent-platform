<template>
  <div class="platform-agent-list">
    <header class="platform-agent-list__header">
      <div>
        <h2>{{ $t('agent.title') }}</h2>
        <p>{{ $t('agent.platformSubtitle') }}</p>
      </div>
      <t-button variant="outline" :loading="loading" @click="loadAgents">
        <template #icon><t-icon name="refresh" /></template>
        {{ $t('common.refresh') }}
      </t-button>
    </header>

    <t-alert v-if="loadError" theme="error" :message="loadError" />
    <div v-if="loading && agents.length === 0" class="platform-agent-list__loading">
      <t-loading size="medium" />
    </div>
    <div v-else class="platform-agent-list__grid">
      <button
        v-for="agent in agents"
        :key="agent.id"
        type="button"
        class="platform-agent-card"
        @click="editAgent(agent)"
      >
        <span class="platform-agent-card__icon">
          <t-icon :name="agent.config?.agent_mode === 'smart-reasoning' ? 'control-platform' : 'chat'" />
        </span>
        <span class="platform-agent-card__body">
          <span class="platform-agent-card__title">{{ agent.name }}</span>
          <span class="platform-agent-card__description">{{ agent.description || $t('agent.noDescription') }}</span>
          <span class="platform-agent-card__badge">{{ $t('agent.platformBuiltin') }}</span>
        </span>
        <t-icon name="edit-1" class="platform-agent-card__edit" />
      </button>
    </div>

    <AgentEditorModal
      :visible="editorVisible"
      mode="edit"
      scope="platform"
      :agent="editingAgent"
      @update:visible="editorVisible = $event"
      @success="handleSaved"
    />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listPlatformAgents, type CustomAgent } from '@/api/agent'
import AgentEditorModal from './AgentEditorModal.vue'

const INTERNAL_AGENT_IDS = new Set(['builtin-skill-installer', 'builtin-wiki-fixer'])

const agents = ref<CustomAgent[]>([])
const loading = ref(false)
const loadError = ref('')
const editorVisible = ref(false)
const editingAgent = ref<CustomAgent | null>(null)

async function loadAgents() {
  loading.value = true
  loadError.value = ''
  try {
    const response = await listPlatformAgents()
    agents.value = (response?.data || []).filter((agent) => !INTERNAL_AGENT_IDS.has(agent.id))
  } catch (error: any) {
    loadError.value = error?.message || String(error)
  } finally {
    loading.value = false
  }
}

function editAgent(agent: CustomAgent) {
  editingAgent.value = agent
  editorVisible.value = true
}

async function handleSaved() {
  await loadAgents()
}

onMounted(loadAgents)
</script>

<style scoped lang="less">
.platform-agent-list {
  min-height: 100%;
  padding: 24px;
}

.platform-agent-list__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 24px;

  h2 { margin: 0 0 8px; font-size: 20px; color: var(--td-text-color-primary); }
  p { margin: 0; color: var(--td-text-color-secondary); }
}

.platform-agent-list__loading {
  display: grid;
  min-height: 240px;
  place-items: center;
}

.platform-agent-list__grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 16px;
  margin-top: 16px;
}

.platform-agent-card {
  display: flex;
  min-height: 132px;
  padding: 18px;
  gap: 14px;
  text-align: left;
  color: inherit;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 10px;
  cursor: pointer;

  &:hover {
    border-color: var(--td-brand-color);
    box-shadow: var(--td-shadow-1);
  }
}

.platform-agent-card__icon {
  display: grid;
  flex: 0 0 40px;
  height: 40px;
  place-items: center;
  color: var(--td-brand-color);
  background: var(--td-brand-color-light);
  border-radius: 10px;
  font-size: 22px;
}

.platform-agent-card__body { display: flex; min-width: 0; flex: 1; flex-direction: column; }
.platform-agent-card__title { font-weight: 600; color: var(--td-text-color-primary); }
.platform-agent-card__description {
  display: -webkit-box;
  margin-top: 8px;
  overflow: hidden;
  color: var(--td-text-color-secondary);
  line-height: 20px;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}
.platform-agent-card__badge {
  align-self: flex-start;
  margin-top: auto;
  padding-top: 12px;
  color: var(--td-brand-color);
  font-size: 12px;
}
.platform-agent-card__edit { flex: 0 0 auto; color: var(--td-text-color-placeholder); }
</style>
