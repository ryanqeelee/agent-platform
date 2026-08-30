<template>
  <div class="runtime-context" role="note" :aria-label="$t('modelSettings.runtimeContext.title')">
    <div class="runtime-context__heading">
      <span class="runtime-context__eyebrow">{{ $t('modelSettings.runtimeContext.eyebrow') }}</span>
      <strong>{{ $t('modelSettings.runtimeContext.title') }}</strong>
    </div>
    <div v-if="settings" class="runtime-context__grid">
      <div>
        <span>{{ $t('modelSettings.runtimeContext.scope') }}</span>
        <strong>{{ $t(`modelSettings.runtimeContext.${settings.scope.kind}`) }}</strong>
      </div>
      <div>
        <span>{{ $t('modelSettings.runtimeContext.activePlan') }}</span>
        <strong>{{ settings.active_plan.version_id }}</strong>
      </div>
      <div>
        <span>{{ $t('modelSettings.runtimeContext.employeeAssistant') }}</span>
        <strong>{{ settings.request_runtime_refs.employee_assistant_request_runtime }}</strong>
      </div>
      <div>
        <span>{{ $t('modelSettings.runtimeContext.operatingAnalysis') }}</span>
        <strong>{{ settings.request_runtime_refs.operating_analysis_request_runtime }}</strong>
      </div>
    </div>
    <p v-else class="runtime-context__unavailable">
      {{ $t('modelSettings.runtimeContext.unavailable') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  getPlatformModelRuntimeSettings,
  type PlatformModelRuntimeSettings,
} from '@/api/model'

const settings = ref<PlatformModelRuntimeSettings | null>(null)

onMounted(async () => {
  try {
    settings.value = await getPlatformModelRuntimeSettings()
  } catch {
    settings.value = null
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
