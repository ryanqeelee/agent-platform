<template>
  <main class="enterprise-home" :aria-busy="loading">
    <header class="page-header">
      <div>
        <p class="eyebrow">{{ copy.eyebrow }}</p>
        <h1>{{ copy.title }}</h1>
        <p class="description">{{ copy.description }}</p>
      </div>
      <span v-if="authStore.selectedTenantName || authStore.tenant?.name" class="tenant-name">
        {{ authStore.selectedTenantName || authStore.tenant?.name }}
      </span>
    </header>

    <nav class="enterprise-settings" :aria-label="copy.eyebrow">
      <button type="button" @click="router.push('/platform/knowledge-bases')">{{ t('menu.knowledgeBase') }}</button>
      <button v-for="entry in managementEntries" :key="entry.section" type="button"
        @click="openSettings(entry.section)">{{ entry.label }}</button>
    </nav>

    <section v-if="queue" class="summary-grid" aria-label="Enterprise summary">
      <article class="summary-card">
        <span>{{ copy.members }}</span>
        <strong>{{ queue.summary.member_usage }}</strong>
      </article>
      <article class="summary-card">
        <span>{{ copy.storage }}</span>
        <strong>{{ storageLabel }}</strong>
      </article>
    </section>

    <section v-if="queue" class="edge-section" aria-labelledby="edge-title">
      <div class="section-heading">
        <div>
          <h2 id="edge-title">{{ copy.edgeTitle }}</h2>
          <p>{{ copy.edgeDescription }}</p>
        </div>
        <button class="refresh" type="button" :disabled="loading" @click="loadQueue">{{ copy.refresh }}</button>
      </div>
      <template v-if="queue.edge_nodes != null">
        <p class="edge-count">{{ copy.nodeCount }} {{ queue.edge_nodes.length }} · {{ copy.availableCount }} {{ availableCount }}</p>
        <div v-if="queue.edge_nodes.length" class="edge-table-wrap">
          <table class="edge-table">
            <thead><tr>
              <th scope="col">{{ copy.edgeNode }}</th><th scope="col">{{ copy.availability }}</th>
              <th scope="col">{{ copy.connection }}</th><th scope="col">{{ copy.dataService }}</th>
              <th scope="col">{{ copy.lastSeen }}</th>
            </tr></thead>
            <tbody>
              <tr v-for="node in queue.edge_nodes" :key="node.edge_node_id">
                <td><strong>{{ copy.edgeNode }}</strong><span class="node-id">{{ node.edge_node_id }}</span>
                  <p v-if="node.availability !== 'available'" class="node-reason">{{ copy.reasons[node.availability] || copy.reasons.unknown }}</p>
                </td>
                <td :data-label="copy.availability"><span class="node-status" :class="`node-status--${node.availability}`">{{ copy.statuses[node.availability] || copy.unknown }}</span></td>
                <td :data-label="copy.connection">{{ copy.statuses[node.connection_status] || copy.unknown }}</td>
                <td :data-label="copy.dataService">{{ copy.statuses[node.data_service_status] || copy.unknown }}</td>
                <td :data-label="copy.lastSeen"><time :datetime="node.last_seen_at || undefined">{{ formatTime(node.last_seen_at) }}</time></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="empty-state"><h3>{{ copy.empty }}</h3><p>{{ copy.emptyDescription }}</p></div>
        <p v-if="queue.edge_nodes.length" class="telemetry-note">{{ copy.telemetryNote }}</p>
      </template>
      <div v-else class="empty-state" role="status"><p>{{ copy.statusUnavailable }}</p></div>
      <p class="updated-at">{{ copy.updatedAt }} {{ formatTime(queue.as_of) }}</p>
    </section>

    <section v-else-if="loading" class="loading-state">
      <span class="loading-line" />
      <span class="loading-line loading-line--short" />
    </section>

    <section v-else class="error-state">
      <h2>{{ copy.loadFailed }}</h2>
      <button type="button" @click="loadQueue">{{ copy.retry }}</button>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import {
  getEnterpriseAdministrationQueue,
  type EnterpriseAdministrationQueueV1,
} from '@/api/enterpriseAdministration'
import { getEnterpriseAdministrationCopy } from '@/config/productShellBrand'
import { useAuthStore } from '@/stores/auth'
import { SETTINGS_SECTION_MIN_ROLE } from '@/config/settingsAccess'

const { t, locale } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const copy = computed(() => getEnterpriseAdministrationCopy(locale.value))
const queue = ref<EnterpriseAdministrationQueueV1>()
const loading = ref(true)
const managementEntries = computed(() => [
  { section: 'members', label: t('tenantMember.title') },
  { section: 'tenant', label: t('settings.tenantInfo') },
  { section: 'businessRoles', label: t('businessRoles.title') },
  { section: 'enterprise-skills', label: t('enterpriseSkills.title') },
  { section: 'memory', label: t('memoryWorkspaceSettings.title') },
].filter(entry => authStore.effectiveCrossTenantAccess
  || authStore.hasRole(SETTINGS_SECTION_MIN_ROLE[entry.section])))

function openSettings(section: string) {
  router.push({ path: '/platform/settings', query: { section } })
}

const availableCount = computed(() => queue.value?.edge_nodes?.filter(node => node.availability === 'available').length ?? 0)

function formatTime(value: string | null) {
  if (!value) return copy.value.noContact
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? copy.value.noContact : date.toLocaleString(locale.value)
}

const storageLabel = computed(() => {
  if (!queue.value) return '—'
  const used = formatBytes(queue.value.summary.storage_usage_bytes)
  const quota = queue.value.summary.storage_quota_bytes
  return quota > 0 ? `${used} / ${formatBytes(quota)}` : used
})

function formatBytes(value: number) {
  if (!value) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const unit = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / 1024 ** unit).toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

async function loadQueue() {
  loading.value = true
  try {
    queue.value = await getEnterpriseAdministrationQueue()
  } catch {
    queue.value = undefined
  } finally {
    loading.value = false
  }
}

onMounted(() => void loadQueue())
</script>

<style scoped lang="less">
.enterprise-home {
  width: min(1120px, calc(100% - 48px));
  min-height: 100%;
  margin: 0 auto;
  padding: 42px 0 72px;
  color: var(--product-shell-ink);
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 32px;
  padding-bottom: 28px;
  border-bottom: 1px solid rgba(19, 45, 45, 0.12);

  h1 { margin: 4px 0 10px; font-size: clamp(28px, 3vw, 36px); line-height: 1.1; letter-spacing: -0.035em; }
}

.enterprise-settings { display: flex; flex-wrap: wrap; gap: 10px; margin: 20px 0; }
.enterprise-settings button { padding: 10px 16px; border: 1px solid var(--td-component-stroke); border-radius: 8px; background: var(--td-bg-color-container); color: var(--product-shell-teal); font: inherit; cursor: pointer; }
.enterprise-settings button:hover { background: var(--td-bg-color-container-hover); }

.eyebrow { margin: 0; color: var(--product-shell-teal); font-size: 13px; font-weight: 700; letter-spacing: .14em; }
.description { max-width: 680px; margin: 0; color: rgba(19, 45, 45, .67); font-size: 15px; line-height: 1.7; }
.tenant-name { padding: 8px 13px; border: 1px solid rgba(19, 45, 45, .12); border-radius: 999px; background: rgba(255, 255, 255, .58); font-size: 13px; font-weight: 650; white-space: nowrap; }

.summary-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-top: 20px; }
.summary-card { padding: 18px; border: 1px solid rgba(19, 45, 45, .11); border-radius: 18px; background: rgba(255, 255, 255, .7); }
.summary-card span { display: block; margin-bottom: 8px; color: rgba(19, 45, 45, .58); font-size: 12px; }
.summary-card strong { font-size: 19px; font-weight: 680; }

.edge-section { margin-top: 34px; }
.section-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; margin-bottom: 14px; }
.section-heading h2 { margin: 0 0 8px; font-size: 24px; }
.section-heading p { margin: 0; color: var(--td-text-color-secondary); line-height: 1.6; }
.refresh, .error-state button { padding: 8px 12px; border: 1px solid var(--td-component-stroke); border-radius: 8px; color: var(--product-shell-teal); background: var(--td-bg-color-container); font: inherit; font-size: 13px; cursor: pointer; white-space: nowrap; }
.refresh:disabled { opacity: .5; cursor: wait; }
button:focus-visible { outline: 2px solid var(--product-shell-teal); outline-offset: 3px; }
.edge-count { color: var(--product-shell-teal); font-size: 13px; }
.edge-table-wrap { overflow-x: auto; border: 1px solid var(--td-component-stroke); border-radius: 14px; }
.edge-table { width: 100%; border-collapse: collapse; text-align: left; background: var(--td-bg-color-container); }
.edge-table th { padding: 14px 18px; color: var(--td-text-color-secondary); font-size: 12px; font-weight: 500; white-space: nowrap; }
.edge-table td { padding: 18px; border-top: 1px solid var(--td-component-stroke); font-size: 14px; vertical-align: top; }
.edge-table td:first-child { min-width: 220px; max-width: 340px; }
.edge-table td:not(:first-child) { min-width: 100px; }
.node-id { display: block; color: var(--td-text-color-secondary); margin-top: 5px; font-size: 12px; overflow-wrap: anywhere; }
.node-reason { color: var(--td-text-color-secondary); font-size: 12px; line-height: 1.6; margin: 8px 0 0; }
.node-status { white-space: nowrap; color: var(--td-text-color-secondary); }
.node-status--available { color: var(--product-shell-teal); }
.node-status--abnormal, .node-status--offline { color: var(--product-shell-risk); }
.empty-state { padding: 36px 24px; border: 1px solid var(--td-component-stroke); border-radius: 14px; text-align: center; background: var(--td-bg-color-container); }
.empty-state h3 { margin: 0 0 12px; font-size: 18px; }
.empty-state p { margin: 0; color: var(--td-text-color-secondary); line-height: 1.6; }
.telemetry-note, .updated-at { font-size: 12px; color: var(--td-text-color-secondary); line-height: 1.6; }

.loading-state, .error-state { margin-top: 34px; padding: 40px; border: 1px solid rgba(19, 45, 45, .11); border-radius: 20px; background: rgba(255, 255, 255, .62); }
.loading-line { display: block; width: 72%; height: 14px; border-radius: 8px; background: rgba(19, 45, 45, .1); animation: pulse 1.2s ease-in-out infinite; }
.loading-line--short { width: 42%; margin-top: 14px; }
.error-state h2 { margin: 0 0 14px; font-size: 20px; }

@keyframes pulse { 50% { opacity: .45; } }

@media (max-width: 820px) {
  .enterprise-home { width: min(100% - 28px, 1120px); padding-top: 28px; }
  .page-header { flex-direction: column; gap: 18px; }
  .summary-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}

@media (max-width: 640px) {
  .edge-table thead { display: none; }
  .edge-table, .edge-table tbody { display: block; }
  .edge-table tr { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); padding: 16px; gap: 16px; }
  .edge-table tr + tr { border-top: 1px solid var(--td-component-stroke); }
  .edge-table td, .edge-table td:first-child, .edge-table td:not(:first-child) { display: block; min-width: 0; max-width: none; padding: 0; border: 0; }
  .edge-table td:first-child, .edge-table td:last-child { grid-column: 1 / -1; }
  .edge-table td[data-label]::before { content: attr(data-label); display: block; margin-bottom: 6px; color: var(--td-text-color-secondary); font-size: 12px; }
}

@media (max-width: 520px) {
  .summary-grid { grid-template-columns: 1fr; }
  .section-heading { flex-direction: column; gap: 12px; }
}

@media (prefers-reduced-motion: reduce) {
  .loading-line { animation: none; }
}
</style>
