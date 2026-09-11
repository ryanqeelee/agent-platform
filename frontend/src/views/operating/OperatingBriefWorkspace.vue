<template>
  <section class="operating-brief-workspace" aria-label="经营简报">
    <div v-if="isDataNotConnected" class="operating-brief operating-brief--not-connected">
      <h1>经营简报</h1>
      <section class="operating-data-connection" role="status">
        <strong>当前企业尚未接入经营数据</strong>
        <p>接入门店经营数据后，这里将展示销售、毛利和经营变化。请联系企业管理员完成数据接入。</p>
        <div class="operating-data-connection__actions">
          <button type="button" :disabled="state.loading" @click="controller.refresh()">
            {{ state.loading ? '正在检查…' : '检查接入状态' }}
          </button>
          <button type="button" :disabled="!active" @click="controller.openAnalysis()">进入经营分析</button>
        </div>
      </section>
    </div>

    <div v-else class="operating-brief">
      <div class="operating-brief__toolbar">
        <label v-if="state.brief">
          查看范围
          <select
            aria-label="查看范围"
            :disabled="state.loading || state.launching"
            :value="state.selectedScopeRef ?? ''"
            @change="selectScope"
          >
            <option
              v-for="option in state.brief.scopeOptions"
              :key="option.scopeRef ?? 'all'"
              :value="option.scopeRef ?? ''"
            >
              {{ option.label }}
            </option>
          </select>
        </label>
        <span v-else>查看范围</span>
        <div class="operating-brief__toolbar-actions">
          <button type="button" :disabled="state.loading || state.launching" @click="controller.refresh()">刷新</button>
          <span :title="state.brief?.weeklyCore.readyAt ?? undefined">
            {{ readyAtLabel }}
          </span>
          <button type="button" :disabled="!active" @click="controller.openAnalysis()">进入经营分析</button>
        </div>
      </div>

      <div v-if="state.error" class="operating-brief__notice" role="status">
        {{ state.error }}
        <button type="button" @click="controller.refresh()">重试</button>
      </div>
      <div
        v-if="state.pollExhausted && state.brief?.displayState === 'preparing'"
        class="operating-brief__notice"
        role="status"
      >
        自动更新已暂停。
        <button type="button" @click="controller.refresh()">重新检查</button>
      </div>

      <div v-if="state.brief" class="operating-brief__surface">
        <section class="operating-brief__hero">
          <div>
            <p class="operating-brief__eyebrow">本周经营简报 · {{ state.brief.selectedScope.label }}</p>
            <h1>本周经营，先看这几件事</h1>
            <p>销售有没有带来更多毛利，变化集中在哪里，下一步先查什么。</p>
          </div>
          <div class="operating-brief__period">
            <strong>
              {{ currentPeriod?.start ?? '本周数据暂不可用' }}{{ currentPeriod?.endExclusive ? ` 至 ${inclusiveEnd(currentPeriod.endExclusive)}` : '' }}
            </strong>
            <small v-if="currentPeriod">同比 {{ briefData?.periods.yoy?.start ?? '—' }} 至 {{ inclusiveEnd(briefData?.periods.yoy?.endExclusive ?? null) }}</small>
            <small v-if="currentPeriod">环比 {{ briefData?.periods.wow?.start ?? '—' }} 至 {{ inclusiveEnd(briefData?.periods.wow?.endExclusive ?? null) }}</small>
          </div>
        </section>

        <section
          v-if="state.brief.displayState !== 'ready'"
          class="operating-brief__state"
          :class="`operating-brief__state--${state.brief.displayState}`"
          role="status"
        >
          <strong>{{ displayStateLabel }}</strong>
          <span>{{ displayStateCopy }}</span>
        </section>

        <section v-if="noticeMessages.length" class="operating-brief__method-notices" aria-label="数据口径说明">
          <h2>数据口径说明</h2>
          <ul>
            <li v-for="(notice, index) in noticeMessages" :key="`${notice.level}-${index}`" :data-level="notice.level">
              {{ notice.message }}
            </li>
          </ul>
        </section>

        <template v-if="briefData">
          <section
            v-if="currentPeriod?.coverage"
            class="operating-brief__coverage-summary"
            :class="{ 'operating-brief__coverage-summary--warning': incompleteCoverage }"
            aria-label="覆盖情况"
          >
            <strong>数据覆盖</strong>
            <span>{{ currentPeriod.coverage.coveredStoreCount ?? '—' }} / {{ currentPeriod.coverage.expectedStoreCount ?? '—' }} 家门店</span>
            <small v-if="incompleteCoverage">覆盖尚不完整，比较、趋势或影响拆分可能受限。</small>
            <small v-else>本周门店覆盖完整。</small>
          </section>

          <section v-if="availableKpis.length" class="operating-brief__kpis" aria-label="关键经营指标">
            <article v-for="kpi in availableKpis" :key="kpi.key" class="operating-brief__card">
              <span>{{ kpi.label }}</span>
              <strong>{{ displayValue(kpi.value, kpi.unit) }}</strong>
              <p class="operating-brief__metric-purpose">{{ metricPurpose(kpi.key) }}</p>
              <div>
                <template v-for="comparison in kpiComparisons(kpi)" :key="comparison.kind">
                  <button
                    v-if="comparison.anchor"
                    type="button"
                    class="operating-brief__comparison"
                    @click="controller.ask(comparison.anchor, comparison.question)"
                  >
                    {{ comparison.text }}
                  </button>
                  <span v-else class="operating-brief__comparison">{{ comparison.text }}</span>
                </template>
              </div>
              <button
                v-if="canLaunch && kpi.observationAnchorRef"
                type="button"
                @click="controller.ask(kpi.observationAnchorRef, `分析${kpi.label}`)"
              >
                发起经营分析
              </button>
            </article>
          </section>

          <div class="operating-brief__grid operating-brief__grid--questions">
            <section class="operating-brief__panel operating-brief__focus">
              <h2>本周优先核查</h2>
              <template v-if="focusItems.length">
                <article v-for="item in focusItems" :key="item.key">
                  <p>{{ item.statement }}</p>
                  <button
                    v-if="canLaunch && item.observationAnchorRef"
                    type="button"
                    @click="controller.ask(item.observationAnchorRef, item.statement)"
                  >
                    展开分析
                  </button>
                </article>
              </template>
              <p v-else class="operating-brief__quiet">当前可用比较未触发组合观察。请继续查看销售与毛利变化；这不表示经营没有问题。</p>
            </section>

            <section class="operating-brief__panel">
              <h2>接下来先看什么</h2>
              <ol v-if="questions.length" class="operating-brief__questions">
                <li v-for="item in questions" :key="item.key">
                  <button
                    v-if="canLaunch && item.observationAnchorRef"
                    type="button"
                    :aria-label="item.question"
                    :aria-describedby="item.reason ? questionReasonId(item.key) : undefined"
                    @click="controller.ask(item.observationAnchorRef, item.reason ? `${item.question}\n${item.reason}` : item.question)"
                  >
                    <strong>{{ item.question }}</strong>
                    <small v-if="item.reason" :id="questionReasonId(item.key)">{{ item.reason }}</small>
                  </button>
                  <span v-else>
                    <strong>{{ item.question }}</strong>
                    <small v-if="item.reason" :id="questionReasonId(item.key)">{{ item.reason }}</small>
                  </span>
                </li>
              </ol>
              <p v-else class="operating-brief__quiet">当前没有可安全展开的建议问题。</p>
            </section>
          </div>

          <div class="operating-brief__grid">
            <section class="operating-brief__panel">
              <h2>{{ isEnterprise ? '先看哪些门店' : '先看哪些品类' }}</h2>
              <p class="operating-brief__quiet">按同比销售减少金额排列；正向增长单独展示，避免被总体数字掩盖。</p>
              <p v-if="salesYoyDelta?.status === 'available'">
                销售同比净变化 <strong>{{ signedValue(salesYoyDelta.value, salesYoyDelta.unit) }}</strong>
              </p>
              <ul v-if="impactItems.length" class="operating-brief__impact-list">
                <li v-for="item in impactItems" :key="`${item.label}-${item.rollup}`">
                  <div class="operating-brief__impact-row">
                    <span>{{ item.label }}{{ item.rollup ? '（汇总）' : '' }}</span>
                    <strong>{{ signedValue(item.amount, 'CNY') }}</strong>
                  </div>
                  <div class="operating-brief__impact-track" aria-hidden="true">
                    <i :class="Number(item.amount) < 0 ? 'is-negative' : 'is-positive'" :style="{ width: impactWidth(item.amount) }" />
                  </div>
                  <div class="operating-brief__impact-actions">
                    <button
                      v-if="matchingStoreScope(item)"
                      type="button"
                      @click="controller.selectScope(matchingStoreScope(item)?.scopeRef ?? null)"
                    >
                      查看{{ item.label }}简报
                    </button>
                    <button
                      v-if="canLaunch && item.observationAnchorRef"
                      type="button"
                      @click="controller.ask(item.observationAnchorRef, `分析${item.label}对同比销售的影响`)"
                    >
                      分析{{ item.label }}的变化
                    </button>
                  </div>
                </li>
              </ul>
              <p v-else class="operating-brief__quiet">
                {{ briefData.salesYoyImpact.status === 'available' ? '当前可比范围内没有非零销售影响项。' : '当前无法提供同比影响拆分，暂不能判断变化集中在哪里。' }}
              </p>
              <small class="operating-brief__quiet">影响金额表示销售变化的贡献，不是原因结论；正向汇总尚不能据此选出标杆门店。</small>
            </section>

            <section class="operating-brief__panel">
              <h2>近 8 周销售趋势</h2>
              <p class="operating-brief__quiet">用连续周数据核查本周变化；单周环比不能直接证明趋势反转。</p>
              <ol v-if="trend.length" class="operating-brief__trend">
                <li v-for="point in trend" :key="point.start ?? 'unknown'">
                  <button
                    v-if="trendHasValue(point) && canLaunch && point.observationAnchorRef"
                    type="button"
                    @click="controller.ask(point.observationAnchorRef, `分析 ${point.start ?? '该周'} 的销售变化`)"
                  >
                    <span>{{ trendLabel(point) }}</span>
                    <strong>{{ displayValue(point.sales.value, point.sales.unit) }}</strong>
                  </button>
                  <span v-else>{{ trendLabel(point) }} · {{ trendHasValue(point) ? displayValue(point.sales.value, point.sales.unit) : '暂不可用' }}</span>
                </li>
              </ol>
              <p v-else class="operating-brief__quiet">当前没有可展示的趋势点。</p>
            </section>
          </div>
        </template>
      </div>

      <section v-else class="operating-brief__surface operating-brief__surface--loading" :aria-busy="state.loading">
        <div class="operating-brief__hero">
          <div>
            <p class="operating-brief__eyebrow">本周经营简报</p>
            <h1>本周经营，先看这几件事</h1>
            <p>{{ state.error ? '可重试加载简报，或切换到经营分析继续提问。' : '数据正在后台读取，你可以先进入经营分析。' }}</p>
          </div>
        </div>
        <p v-if="!state.error" class="operating-brief__state" role="status">正在读取经营数据…</p>
      </section>

      <div v-if="state.pending" class="operating-brief__confirm" role="dialog" aria-modal="true" aria-label="确认启动经营分析">
        <div>
          <h2>进入经营分析？</h2>
          <p>将围绕这条已确认的经营观察启动一次分析，沿用所选范围与数据期间。</p>
          <p>{{ state.brief?.selectedScope.label }} · {{ operatingBriefPeriodLabel }}</p>
          <p v-if="state.error" role="alert">{{ state.error }} 请取消后刷新推荐，或重试确认。</p>
          <p class="operating-brief__selected-question">{{ state.pending.question }}</p>
          <footer>
            <button type="button" :disabled="state.launching" @click="controller.cancelPending()">取消</button>
            <button type="button" :disabled="state.launching || state.loading" @click="controller.launch()">
              {{ state.launching ? '正在启动…' : '确认分析' }}
            </button>
          </footer>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  createCurrentUserOperatingBriefClient,
  type OperatingBriefComparisonDTO,
  type OperatingBriefKpiDTO,
} from '@/api/operatingBrief'
import { OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY } from '@/api/operatingAnalysis'
import { useAuthStore } from '@/stores/auth'
import {
  createOperatingBriefController,
  createOperatingBriefState,
} from './operatingBriefController'

const props = defineProps<{ active: boolean }>()
const auth = useAuthStore()
const router = useRouter()
const state = reactive(createOperatingBriefState())
const active = computed(() => props.active)

const controller = createOperatingBriefController({
  state,
  source: createCurrentUserOperatingBriefClient(() => ({
    token: auth.token,
    tenantId: String(auth.effectiveTenantId ?? ''),
  })),
  active: props.active,
  visible: typeof document === 'undefined' || document.visibilityState !== 'hidden',
  onStartAnalysis(handoff) {
    sessionStorage.setItem(OPERATING_ANALYSIS_HANDOFF_PROMPT_KEY, JSON.stringify(handoff))
    void router.push('/platform/operating-analysis')
  },
  onOpenAnalysis() {
    void router.push('/platform/operating-analysis')
  },
})

const briefData = computed(() => state.brief?.weeklyCore.data ?? null)
const currentPeriod = computed(() => briefData.value?.periods.current)
const incompleteCoverage = computed(() => currentPeriod.value?.coverage.status !== 'available')
const canLaunch = computed(() => Boolean(state.brief?.briefSnapshotRef))
const focusItems = computed(() => briefData.value?.focusItems.slice(0, 3) ?? [])
const questions = computed(() => briefData.value?.suggestedQuestions ?? [])
const trend = computed(() => briefData.value?.salesTrend ?? [])
const availableKpis = computed(() => (briefData.value?.kpis ?? []).filter(
  kpi => kpi.value !== null && (kpi.status === 'available' || kpi.status === 'partial'),
))
const impactItems = computed(() => briefData.value?.salesYoyImpact.status === 'available'
  ? briefData.value.salesYoyImpact.items ?? []
  : [])
const maxImpact = computed(() => Math.max(1, ...impactItems.value.map(item => Math.abs(Number(item.amount)))))
const isEnterprise = computed(() => state.brief?.selectedScope.kind === 'all_operating_stores')
const salesYoyDelta = computed(() => briefData.value?.kpis.find(kpi => kpi.key === 'net_sales_amount')?.yoy.delta)
const noticeMessages = computed(() => (state.brief?.notices ?? []).filter(
  (notice): notice is { level?: string; message: string } => typeof notice.message === 'string' && Boolean(notice.message.trim()),
))
const isDataNotConnected = computed(() => !state.error && state.brief?.weeklyCore.reasonCode === 'business_data_not_connected')

const readyAtLabel = computed(() => {
  if (state.loading) return '正在更新…'
  const value = state.brief?.weeklyCore.readyAt
  if (!value) return '数据发布时间暂不可用'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '数据发布时间暂不可用'
  return `数据发布于 ${date.toLocaleString('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false,
  })}`
})

const displayStateLabel = computed(() => {
  switch (state.brief?.displayState) {
    case 'partial': return '部分可用'
    case 'preparing': return '正在准备'
    case 'no_data': return '暂无本周数据'
    default: return '暂不可用'
  }
})

const displayStateCopy = computed(() => {
  switch (state.brief?.displayState) {
    case 'preparing': return '本周已结束，正在等待正式周报发布。发布完成后将自动更新。'
    case 'no_data': return '当前范围没有可展示的官方本周数值；不会将缺失事实显示为零。'
    case 'unavailable': return '当前无法验证经营事实，暂不展示推断数值。'
    default: return '部分经营事实尚未齐备；已确认的数值保留展示，未确认部分不会补零。'
  }
})

const operatingBriefPeriodLabel = computed(() => {
  const period = currentPeriod.value
  if (!period?.start || !period.endExclusive) return ''
  return `${period.start} 至 ${inclusiveEnd(period.endExclusive)}`
})

function displayValue(value: string | null, unit: string | null): string {
  if (value === null) return '暂不可用'
  const number = Number(value)
  if (!Number.isFinite(number)) return '暂不可用'
  if (unit === 'CNY') {
    const absolute = Math.abs(number)
    return absolute >= 10_000
      ? `${(number / 10_000).toLocaleString('zh-CN', { maximumFractionDigits: 2 })} 万元`
      : `${number.toLocaleString('zh-CN', { maximumFractionDigits: 2 })} 元`
  }
  if (unit === 'ratio') return `${(number * 100).toLocaleString('zh-CN', { maximumFractionDigits: 2 })}%`
  if (unit === 'percentage_point') return `${number.toLocaleString('zh-CN', { maximumFractionDigits: 2 })} 个百分点`
  return value
}

function signedValue(value: string | null, unit: string | null): string {
  const rendered = displayValue(value, unit)
  return value !== null && Number(value) > 0 ? `+${rendered}` : rendered
}

function inclusiveEnd(value: string | null): string {
  if (!value) return '—'
  const date = new Date(`${value}T00:00:00Z`)
  date.setUTCDate(date.getUTCDate() - 1)
  return date.toISOString().slice(0, 10)
}

function comparison(
  label: string,
  values: Record<string, OperatingBriefComparisonDTO>,
  question: string,
) {
  const relative = values.relativeChange ?? values.deltaPercentagePoints
  const delta = values.delta
  const available = (value?: OperatingBriefComparisonDTO) => Boolean(
    value && (value.status === 'available' || value.status === 'partial') && value.value !== null,
  )
  const primary = available(relative) ? relative : available(delta) ? delta : null
  if (!primary) return { text: `${label} 暂不可比`, anchor: null, question }
  const secondary = primary !== delta && available(delta) ? ` · ${signedValue(delta.value, delta.unit)}` : ''
  const anchor = primary.observationAnchorRef ?? (available(delta) ? delta.observationAnchorRef : null)
  return { text: `${label} ${signedValue(primary.value, primary.unit)}${secondary}`, anchor, question }
}

function kpiComparisons(kpi: OperatingBriefKpiDTO) {
  return [
    { kind: 'yoy', ...comparison('同比', kpi.yoy, `分析${kpi.label}的同比变化`) },
    { kind: 'wow', ...comparison('环比', kpi.wow, `分析${kpi.label}的环比变化`) },
  ]
}

function metricPurpose(key: string): string {
  if (key === 'net_sales_amount') return '看经营规模'
  if (key === 'margin_amount') return '看留下多少毛利，不等于净利润'
  return '看每一元销售的毛利水平'
}

function matchingStoreScope(item: { label: string; rollup: boolean }) {
  if (!isEnterprise.value || item.rollup || !state.brief) return null
  const matches = state.brief.scopeOptions.filter(option => option.kind === 'store' && option.label === item.label)
  return matches.length === 1 ? matches[0] : null
}

function impactWidth(amount: string | null): string {
  return `${Math.abs(Number(amount)) / maxImpact.value * 100}%`
}

function trendHasValue(point: NonNullable<typeof briefData.value>['salesTrend'][number]): boolean {
  return point.sales.value !== null && (point.sales.status === 'available' || point.sales.status === 'partial')
}

function trendLabel(point: NonNullable<typeof briefData.value>['salesTrend'][number]): string {
  return `${point.start ?? '期间暂不可用'} 至 ${inclusiveEnd(point.endExclusive)}`
}

function questionReasonId(key: string): string {
  return `operating-brief-question-reason-${key}`
}

function selectScope(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  void controller.selectScope(value || null)
}

function onVisibilityChange() {
  controller.setVisible(document.visibilityState !== 'hidden')
}

watch([() => auth.user?.id, () => auth.effectiveTenantId], () => {
  void controller.resetIdentity()
}, { immediate: true, flush: 'sync' })
watch(() => props.active, value => controller.setActive(value))

if (typeof document !== 'undefined') document.addEventListener('visibilitychange', onVisibilityChange)
onBeforeUnmount(() => {
  if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', onVisibilityChange)
  controller.dispose()
})
</script>

<style scoped>
.operating-brief-workspace {
  --brief-bg: var(--td-bg-color-container);
  --brief-surface: var(--td-bg-color-container);
  --brief-surface-muted: var(--td-bg-color-container-hover);
  --brief-border: var(--td-component-border);
  --brief-text: var(--td-text-color-primary);
  --brief-muted: var(--td-text-color-secondary);
  --brief-brand: var(--td-brand-color);
  --brief-brand-soft: var(--td-brand-color-light);
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow: auto;
  color: var(--brief-text);
  background: var(--brief-bg);
  font-variant-numeric: tabular-nums;
}

.operating-brief-workspace * { box-sizing: border-box; }
.operating-brief { min-height: 100%; padding: 28px 36px; }
.operating-brief__surface { max-width: 1120px; margin: 0 auto 40px; }
.operating-brief__toolbar { display: flex; max-width: 1120px; margin: 0 auto 16px; align-items: center; justify-content: space-between; gap: 12px; color: var(--brief-muted); font-size: 13px; }
.operating-brief__toolbar-actions { display: flex; align-items: center; justify-content: flex-end; gap: 12px; }
.operating-brief__toolbar-actions button,
.operating-data-connection button { padding: 7px 12px; border: 1px solid var(--brief-border); border-radius: 7px; background: var(--brief-surface); color: var(--brief-brand); }
.operating-brief select { margin-left: 8px; padding: 7px 9px; border: 1px solid var(--brief-border); border-radius: 7px; background: var(--brief-surface); color: var(--brief-text); font: inherit; }
.operating-brief__hero { display: flex; padding: 22px 0 28px; align-items: end; justify-content: space-between; gap: 24px; border-bottom: 1px solid var(--brief-border); }
.operating-brief__eyebrow { margin: 0 0 9px; color: var(--brief-brand); font-size: 13px; font-weight: 650; }
.operating-brief h1 { margin: 0; font-size: clamp(26px, 3vw, 36px); font-weight: 600; letter-spacing: -.035em; }
.operating-brief__hero p:not(.operating-brief__eyebrow) { max-width: 560px; color: var(--brief-muted); line-height: 1.7; }
.operating-brief__period { display: grid; min-width: 240px; padding: 14px; gap: 8px; border-left: 1px solid var(--brief-border); }
.operating-brief__period small { color: var(--brief-muted); font-size: 12px; }
.operating-brief__coverage-summary,
.operating-brief__card,
.operating-brief__panel,
.operating-brief__method-notices { border: 1px solid var(--brief-border); border-radius: 12px; background: var(--brief-surface); }
.operating-brief__coverage-summary { display: flex; margin: 14px 0 20px; padding: 13px 0; align-items: baseline; gap: 10px; border: 0; background: transparent; }
.operating-brief__coverage-summary strong { color: var(--brief-muted); font-size: 12px; font-weight: 650; }
.operating-brief__coverage-summary span { font-size: 16px; font-weight: 700; }
.operating-brief__coverage-summary small { margin-left: auto; color: var(--brief-muted); }
.operating-brief__coverage-summary--warning { padding-inline: 15px; border: 1px solid color-mix(in srgb, var(--td-warning-color) 55%, var(--brief-border)); background: color-mix(in srgb, var(--td-warning-color) 8%, var(--brief-surface)); }
.operating-brief__method-notices { margin-top: 14px; padding: 16px 18px; }
.operating-brief__method-notices h2 { margin-bottom: 8px; }
.operating-brief__method-notices ul { margin: 0; padding-left: 20px; color: var(--brief-muted); font-size: 13px; line-height: 1.65; }
.operating-brief__kpis { display: grid; margin-top: 14px; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
.operating-brief__card { padding: 22px; }
.operating-brief__card > span { color: var(--brief-muted); font-size: 12px; font-weight: 650; }
.operating-brief__card strong { display: block; margin: 10px 0 13px; font-size: 28px; font-weight: 600; letter-spacing: -.025em; }
.operating-brief__card > div { display: grid; gap: 5px; }
.operating-brief__metric-purpose { margin: 0 0 12px; color: var(--brief-muted); font-size: 12px; }
.operating-brief__comparison { display: flex; width: 100%; padding: 6px 0; align-items: center; justify-content: space-between; gap: 8px; border: 0; border-radius: 5px; background: transparent; color: var(--brief-muted); font-size: 12px; text-align: left; }
button.operating-brief__comparison:hover { background: var(--brief-brand-soft); color: var(--brief-brand); }
.operating-brief__card > button { margin-top: 14px; padding: 0; border: 0; background: none; color: var(--brief-brand); }
.operating-brief__grid { display: grid; margin-top: 14px; grid-template-columns: 1fr 1fr; gap: 14px; }
.operating-brief__panel { padding: 20px; }
.operating-brief h2 { margin: 0 0 14px; font-size: 16px; }
.operating-brief__focus article { display: flex; padding: 12px 0; align-items: center; justify-content: space-between; gap: 14px; border-top: 1px solid var(--brief-border); }
.operating-brief__focus p { margin: 0; line-height: 1.55; }
.operating-brief button { font: inherit; cursor: pointer; }
.operating-brief button:disabled { cursor: not-allowed; opacity: .5; }
.operating-brief__focus button,
.operating-brief__questions button,
.operating-brief__impact-list button { border: 0; background: none; color: var(--brief-brand); text-align: left; }
.operating-brief__focus button { flex-shrink: 0; font-size: 12px; }
.operating-brief__questions,
.operating-brief__impact-list,
.operating-brief__trend { margin: 0; padding: 0; list-style: none; }
.operating-brief__questions li { padding: 10px 0; border-top: 1px solid var(--brief-border); line-height: 1.5; }
.operating-brief__questions button,
.operating-brief__questions li > span { display: grid; gap: 4px; }
.operating-brief__questions strong { font-weight: 650; }
.operating-brief__questions small { color: var(--brief-muted); font-size: 12px; font-weight: 400; line-height: 1.5; }
.operating-brief__impact-list li { display: block; padding: 10px 0; border-top: 1px solid var(--brief-border); }
.operating-brief__impact-row { display: flex; justify-content: space-between; gap: 12px; }
.operating-brief__impact-track { height: 5px; margin: 10px 0; overflow: hidden; border-radius: 3px; background: var(--brief-surface-muted); }
.operating-brief__impact-track i { display: block; height: 100%; }
.operating-brief__impact-track .is-negative { background: var(--td-warning-color); }
.operating-brief__impact-track .is-positive { background: var(--brief-brand); }
.operating-brief__impact-actions { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 12px; font-size: 12px; }
.operating-brief__impact-actions button { padding: 5px 0; }
.operating-brief__trend li { display: flex; padding: 0; justify-content: space-between; gap: 12px; border-top: 1px solid var(--brief-border); }
.operating-brief__trend button { display: flex; width: 100%; padding: 10px 0; justify-content: space-between; gap: 12px; border: 0; background: none; color: var(--brief-text); text-align: left; }
.operating-brief__trend li > span { padding: 10px 0; }
.operating-brief__trend strong { color: var(--brief-brand); }
.operating-brief__quiet { color: var(--brief-muted); }
.operating-brief__notice,
.operating-brief__state { max-width: 700px; margin: 25px auto; padding: 15px; border: 1px solid color-mix(in srgb, var(--td-warning-color) 55%, var(--brief-border)); border-radius: 10px; background: color-mix(in srgb, var(--td-warning-color) 8%, var(--brief-surface)); color: var(--brief-text); }
.operating-brief__notice button { margin-left: 8px; border: 0; background: none; color: inherit; text-decoration: underline; }
.operating-brief__state { display: grid; gap: 7px; }
.operating-brief__state--preparing { border-color: var(--brief-brand); background: var(--brief-surface-muted); }
.operating-brief__state--no_data,
.operating-brief__state--unavailable { background: var(--brief-surface); }
.operating-brief__surface--loading { max-width: 1120px; margin: 0 auto 40px; }
.operating-brief__confirm { position: fixed; z-index: 1000; inset: 0; display: grid; padding: 20px; place-items: center; background: rgba(5, 31, 30, .42); }
.operating-brief__confirm > div { width: min(500px, 100%); padding: 22px; border-radius: 14px; background: var(--brief-surface); box-shadow: var(--td-shadow-3); }
.operating-brief__confirm p { color: var(--brief-muted); line-height: 1.6; }
.operating-brief__confirm footer { display: flex; margin-top: 18px; justify-content: flex-end; gap: 9px; }
.operating-brief__confirm footer button { padding: 8px 13px; border: 1px solid var(--brief-border); border-radius: 7px; background: var(--brief-surface-muted); color: var(--brief-text); }
.operating-brief__confirm footer button:last-child { border-color: var(--brief-brand); background: var(--brief-brand); color: white; }
.operating-brief__selected-question { margin: 14px 0 0; padding: 12px; border-radius: 8px; background: var(--brief-surface-muted); color: var(--brief-text) !important; white-space: pre-line; }
.operating-data-connection { padding: 24px; border: 1px solid var(--brief-border); border-radius: 12px; background: var(--brief-surface-muted); line-height: 1.65; }
.operating-data-connection strong { font-size: 17px; font-weight: 600; }
.operating-data-connection p { margin: 10px 0 0; color: var(--brief-muted); font-size: 14px; }
.operating-data-connection__actions { display: flex; margin-top: 16px; gap: 10px; }
.operating-data-connection__actions button { margin: 0; font: inherit; cursor: pointer; }
.operating-brief--not-connected { max-width: 1120px; margin: 0 auto; }
.operating-brief--not-connected h1 { margin: 24px 0; font-size: 28px; }
.operating-brief button:focus-visible,
.operating-brief select:focus-visible { outline: 2px solid var(--brief-brand); outline-offset: 3px; }

@media (max-width: 760px) {
  .operating-brief { padding: 18px; }
  .operating-brief__hero,
  .operating-brief__grid { display: grid; grid-template-columns: 1fr; }
  .operating-brief__period { border-top: 1px solid var(--brief-border); border-left: 0; }
  .operating-brief__toolbar { align-items: flex-start; flex-direction: column; }
  .operating-brief__toolbar-actions { width: 100%; justify-content: space-between; }
  .operating-brief__kpis { grid-template-columns: 1fr; }
  .operating-brief__coverage-summary { flex-wrap: wrap; }
  .operating-brief__coverage-summary small { width: 100%; margin: 0; }
}
</style>
