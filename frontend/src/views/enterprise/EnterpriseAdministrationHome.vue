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
      <button v-for="entry in managementEntries" :key="entry.section" type="button"
        @click="openSettings(entry.section)">{{ entry.label }}</button>
    </nav>

    <section v-if="queue" class="summary-grid" aria-label="Enterprise summary">
      <article class="summary-card">
        <span>{{ copy.serviceLevel }}</span>
        <strong>{{ queue.summary.service_level || copy.unknown }}</strong>
      </article>
      <article class="summary-card">
        <span>{{ copy.serviceStatus }}</span>
        <strong :class="statusClass">{{ statusLabel }}</strong>
      </article>
      <article class="summary-card">
        <span>{{ copy.members }}</span>
        <strong>{{ quotaLabel(queue.summary.member_usage, queue.summary.member_quota) }}</strong>
      </article>
      <article class="summary-card">
        <span>{{ copy.storage }}</span>
        <strong>{{ storageLabel }}</strong>
      </article>
    </section>

    <section v-if="queue" class="queue-section" aria-labelledby="enterprise-queue-title">
      <div class="section-heading">
        <div>
          <h2 id="enterprise-queue-title">{{ copy.queueTitle }}</h2>
          <p>{{ copy.queueDescription }}</p>
        </div>
        <span class="queue-count">{{ queue.items.length }}</span>
      </div>

      <div v-if="queue.items.length" class="queue-list">
        <article v-for="item in queue.items" :key="item.code" class="queue-item">
          <span class="priority-dot" :class="`priority-dot--${item.priority}`" aria-hidden="true" />
          <div class="item-copy">
            <div class="item-title-row">
              <h3>{{ itemCopy(item.code).title }}</h3>
              <span class="item-count">{{ item.count }}</span>
            </div>
            <p>{{ itemCopy(item.code).description }}</p>
          </div>
          <button type="button" class="item-action" @click="openTarget(item.target)">
            {{ itemCopy(item.code).action }}
            <span aria-hidden="true">→</span>
          </button>
        </article>
      </div>
      <div v-else class="empty-state">
        <span aria-hidden="true">✓</span>
        <p>{{ copy.queueEmpty }}</p>
      </div>
    </section>

    <section v-if="queue" id="service-health" class="service-health" tabindex="-1">
      <div>
        <span>{{ copy.security }}</span>
        <h2>{{ copy.serviceHealthTitle }}</h2>
        <p>{{ copy.serviceHealthDescription }}</p>
      </div>
      <div class="health-result">
        <strong :class="statusClass">{{ statusLabel }}</strong>
        <span v-if="queue.summary.health !== 'healthy'">{{ copy.serviceHealthAction }}</span>
      </div>
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
  type EnterpriseAdministrationItemCode,
  type EnterpriseAdministrationTarget,
} from '@/api/enterpriseAdministration'
import { getEnterpriseAdministrationCopy } from '@/config/productShellBrand'
import { useAuthStore } from '@/stores/auth'
import { useUIStore } from '@/stores/ui'
import { SETTINGS_SECTION_MIN_ROLE } from '@/config/settingsAccess'

const { t, locale } = useI18n()
const router = useRouter()
const authStore = useAuthStore()
const uiStore = useUIStore()
const copy = computed(() => getEnterpriseAdministrationCopy(locale.value))
const queue = ref<EnterpriseAdministrationQueueV1>()
const loading = ref(true)
const managementEntries = computed(() => [
  { section: 'tenant', label: t('settings.tenantInfo') },
  { section: 'members', label: t('tenantMember.title') },
  { section: 'businessRoles', label: t('businessRoles.title') },
  { section: 'memory', label: t('memoryWorkspaceSettings.title') },
].filter(entry => authStore.effectiveCrossTenantAccess
  || authStore.hasRole(SETTINGS_SECTION_MIN_ROLE[entry.section])))

function openSettings(section: string) {
  uiStore.openSettings(section)
  router.push({ path: '/platform/settings', query: { section } })
}

const statusLabel = computed(() => {
  if (!queue.value) return copy.value.unknown
  if (queue.value.summary.health === 'healthy' && queue.value.summary.status === 'active') return copy.value.healthy
  if (queue.value.summary.status === 'unavailable') return copy.value.unavailable
  if (queue.value.summary.health === 'unknown') return copy.value.unknown
  return copy.value.attention
})

const statusClass = computed(() => ({
  'status--healthy': statusLabel.value === copy.value.healthy,
  'status--attention': statusLabel.value === copy.value.attention,
}))

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

function quotaLabel(usage: number, quota?: number) {
  return quota && quota > 0 ? `${usage} / ${quota}` : String(usage)
}

function itemCopy(code: EnterpriseAdministrationItemCode) {
  return copy.value.itemCopy[code]
}

function openTarget(target: EnterpriseAdministrationTarget) {
  if (target === 'service_health') {
    document.getElementById('service-health')?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    return
  }
  if (target === 'knowledge') {
    router.push('/platform/knowledge-bases')
    return
  }
  uiStore.openSettings('members')
  router.push({
    path: '/platform/settings',
    query: target === 'audit' ? { section: 'members', audit: '1' } : { section: 'members' },
  })
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

  h1 { margin: 4px 0 10px; font-size: clamp(32px, 4vw, 48px); line-height: 1.1; letter-spacing: -0.035em; }
}

.enterprise-settings { display: flex; flex-wrap: wrap; gap: 10px; margin: 20px 0; }
.enterprise-settings button { padding: 10px 16px; border: 1px solid var(--td-component-stroke); border-radius: 8px; background: var(--td-bg-color-container); color: var(--product-shell-teal); font: inherit; cursor: pointer; }
.enterprise-settings button:hover { background: var(--td-bg-color-container-hover); }

.eyebrow { margin: 0; color: var(--product-shell-teal); font-size: 13px; font-weight: 700; letter-spacing: .14em; }
.description { max-width: 680px; margin: 0; color: rgba(19, 45, 45, .67); font-size: 15px; line-height: 1.7; }
.tenant-name { padding: 8px 13px; border: 1px solid rgba(19, 45, 45, .12); border-radius: 999px; background: rgba(255, 255, 255, .58); font-size: 13px; font-weight: 650; white-space: nowrap; }

.summary-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; margin-top: 20px; }
.summary-card { padding: 18px; border: 1px solid rgba(19, 45, 45, .11); border-radius: 18px; background: rgba(255, 255, 255, .7); }
.summary-card span { display: block; margin-bottom: 8px; color: rgba(19, 45, 45, .58); font-size: 12px; }
.summary-card strong { font-size: 19px; font-weight: 680; }

.queue-section { margin-top: 34px; }
.section-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 20px; margin-bottom: 14px; }
.section-heading h2, .service-health h2 { margin: 0 0 6px; font-size: 24px; }
.section-heading p, .service-health p { margin: 0; color: rgba(19, 45, 45, .62); line-height: 1.6; }
.queue-count { display: grid; width: 34px; height: 34px; place-items: center; border-radius: 50%; color: #fff; background: var(--product-shell-teal); font-weight: 700; }
.queue-list { overflow: hidden; border: 1px solid rgba(19, 45, 45, .11); border-radius: 20px; background: rgba(255, 255, 255, .72); }
.queue-item { display: grid; grid-template-columns: 10px minmax(0, 1fr) auto; align-items: center; gap: 16px; padding: 19px 21px; border-bottom: 1px solid rgba(19, 45, 45, .09); }
.queue-item:last-child { border-bottom: 0; }
.priority-dot { width: 8px; height: 8px; border-radius: 50%; background: #d49b3e; }
.priority-dot--critical { background: var(--product-shell-risk); }
.priority-dot--high { background: #d58238; }
.item-title-row { display: flex; align-items: center; gap: 10px; }
.item-copy h3 { margin: 0; font-size: 16px; }
.item-copy p { margin: 5px 0 0; color: rgba(19, 45, 45, .6); font-size: 13px; }
.item-count { display: inline-grid; min-width: 23px; height: 23px; padding: 0 7px; place-items: center; border-radius: 999px; color: var(--product-shell-teal); background: rgba(20, 123, 118, .1); font-size: 12px; font-weight: 700; }
.item-action, .error-state button { border: 0; color: var(--product-shell-teal); background: transparent; font: inherit; font-size: 13px; font-weight: 700; cursor: pointer; }
.item-action span { margin-left: 6px; }
.empty-state { display: grid; min-height: 180px; place-items: center; align-content: center; gap: 10px; border: 1px solid rgba(19, 45, 45, .11); border-radius: 20px; background: rgba(255, 255, 255, .62); }
.empty-state span { display: grid; width: 38px; height: 38px; place-items: center; border-radius: 50%; color: #fff; background: var(--product-shell-teal); }
.empty-state p { margin: 0; color: rgba(19, 45, 45, .62); }

.service-health { display: flex; align-items: center; justify-content: space-between; gap: 30px; margin-top: 26px; padding: 24px; border-radius: 22px; color: #f8fbf8; background: #123f3e; }
.service-health > div:first-child > span { display: block; margin-bottom: 7px; color: rgba(248, 251, 248, .58); font-size: 12px; font-weight: 700; letter-spacing: .09em; }
.service-health p { max-width: 680px; color: rgba(248, 251, 248, .68); }
.health-result { text-align: right; white-space: nowrap; }
.health-result strong { display: block; font-size: 18px; }
.health-result span { color: rgba(248, 251, 248, .62); font-size: 12px; }
.status--healthy { color: #2d9b72; }
.service-health .status--healthy { color: #8ce0bd; }
.status--attention { color: var(--product-shell-risk); }

.loading-state, .error-state { margin-top: 34px; padding: 40px; border: 1px solid rgba(19, 45, 45, .11); border-radius: 20px; background: rgba(255, 255, 255, .62); }
.loading-line { display: block; width: 72%; height: 14px; border-radius: 8px; background: rgba(19, 45, 45, .1); animation: pulse 1.2s ease-in-out infinite; }
.loading-line--short { width: 42%; margin-top: 14px; }
.error-state h2 { margin: 0 0 14px; font-size: 20px; }

@keyframes pulse { 50% { opacity: .45; } }

@media (max-width: 820px) {
  .enterprise-home { width: min(100% - 28px, 1120px); padding-top: 28px; }
  .page-header { flex-direction: column; gap: 18px; }
  .summary-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .queue-item { grid-template-columns: 10px minmax(0, 1fr); }
  .item-action { grid-column: 2; justify-self: start; padding: 0; }
  .service-health { align-items: flex-start; flex-direction: column; }
  .health-result { text-align: left; }
}

@media (max-width: 520px) {
  .summary-grid { grid-template-columns: 1fr; }
}

@media (prefers-reduced-motion: reduce) {
  .loading-line { animation: none; }
}
</style>
