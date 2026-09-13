<template>
  <div class="runtime-context" role="note" :aria-label="$t(titleKey)">
    <div class="runtime-context__heading">
      <span class="runtime-context__eyebrow">{{ $t('modelSettings.runtimeContext.eyebrow') }}</span>
      <strong>{{ $t(titleKey) }}</strong>
    </div>
    <div v-if="available" class="runtime-context__grid">
      <div>
        <span>{{ $t('modelSettings.runtimeContext.scope') }}</span>
        <strong>{{ $t(`modelSettings.runtimeContext.${scopeKind}`) }}</strong>
      </div>
      <div>
        <span>{{ $t('modelSettings.runtimeContext.activePlan') }}</span>
        <strong>{{ planVersion }}</strong>
      </div>
      <div v-for="item in capabilityItems" :key="item.labelKey">
        <span>{{ $t(item.labelKey) }}</span>
        <strong>{{ item.value }}</strong>
      </div>
    </div>
    <p v-else class="runtime-context__unavailable">
      {{ $t(unavailableKey) }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  getPlatformModelRuntimeSettings,
} from '@/api/model'
import { getPlatformRetrievalProcessingSettings } from '@/api/retrieval'

const props = withDefaults(defineProps<{ context?: 'request' | 'retrieval' }>(), {
  context: 'request',
})
const available = ref(false)
const scopeKind = ref<'platform_shared' | 'enterprise_assigned'>('platform_shared')
const planVersion = ref('')
const capabilityItems = ref<Array<{ labelKey: string; value: string }>>([])
const titleKey = computed(() => props.context === 'retrieval'
  ? 'modelSettings.runtimeContext.retrievalTitle'
  : 'modelSettings.runtimeContext.title')
const unavailableKey = computed(() => props.context === 'retrieval'
  ? 'modelSettings.runtimeContext.retrievalUnavailable'
  : 'modelSettings.runtimeContext.unavailable')

onMounted(async () => {
  try {
    if (props.context === 'retrieval') {
      const settings = await getPlatformRetrievalProcessingSettings()
      scopeKind.value = settings.scope.kind
      planVersion.value = settings.active_plan.version_id
      capabilityItems.value = [
        { labelKey: 'modelSettings.runtimeContext.embedding', value: settings.capability_refs.embedding },
        { labelKey: 'modelSettings.runtimeContext.reranking', value: settings.capability_refs.reranking },
        { labelKey: 'modelSettings.runtimeContext.parsing', value: settings.capability_refs.parsing },
      ]
    } else {
      const settings = await getPlatformModelRuntimeSettings()
      scopeKind.value = settings.scope.kind
      planVersion.value = settings.active_plan.version_id
      capabilityItems.value = [
        { labelKey: 'modelSettings.runtimeContext.employeeAssistant', value: settings.request_runtime_refs.employee_assistant_request_runtime },
        { labelKey: 'modelSettings.runtimeContext.operatingAnalysis', value: settings.request_runtime_refs.operating_analysis_request_runtime },
      ]
    }
    available.value = true
  } catch {
    available.value = false
  }
})
</script>

<style scoped>
.runtime-context {
  margin-bottom: 20px;
  padding: 16px 18px;
  border: 1px solid var(--td-component-border);
  border-radius: 10px;
  background: var(--td-bg-color-container-hover);
}

.runtime-context__heading {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.runtime-context__eyebrow,
.runtime-context__grid span {
  color: var(--td-text-color-secondary);
  font-size: 12px;
}

.runtime-context__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 24px;
}

.runtime-context__grid > div {
  display: grid;
  gap: 4px;
  min-width: 0;
}

.runtime-context__grid strong {
  overflow-wrap: anywhere;
  font-size: 13px;
  font-weight: 500;
}

.runtime-context__unavailable {
  margin: 0;
  color: var(--td-text-color-secondary);
  font-size: 13px;
}

@media (max-width: 720px) {
  .runtime-context__grid { grid-template-columns: 1fr; }
}
</style>
