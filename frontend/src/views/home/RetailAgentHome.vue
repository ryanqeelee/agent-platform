<template>
  <div class="retail-home">
    <header class="home-nav">
      <RouterLink class="brand-link" to="/home" :aria-label="productShellBrand.name">
        <img src="@/assets/img/product-brand.svg" :alt="productShellBrand.name" />
      </RouterLink>
      <div class="home-account">
        <UserMenu />
      </div>
    </header>

    <main class="home-main">
      <section class="hero" aria-labelledby="home-title">
        <p class="hero-eyebrow">{{ copy.eyebrow }}</p>
        <h1 id="home-title">
          <span class="headline-wide">{{ copy.headline }}</span>
          <span class="headline-compact">{{ copy.compactHeadline }}</span>
        </h1>
        <p class="hero-description">{{ copy.description }}</p>

        <div class="operating-loop" aria-label="Retail operating loop">
          <template v-for="(step, index) in copy.loopSteps" :key="step">
            <span class="loop-step">{{ step }}</span>
            <span v-if="index < copy.loopSteps.length - 1" class="loop-arrow" aria-hidden="true">→</span>
          </template>
        </div>
      </section>

      <section class="work-section" aria-labelledby="current-work-title">
        <div class="section-heading">
          <div>
            <p class="section-kicker">{{ copy.currentWork }}</p>
            <h2 id="current-work-title">{{ copy.loopTitle }}</h2>
          </div>
          <p>{{ copy.loopDescription }}</p>
        </div>

        <div
          class="work-grid"
          :class="{ 'work-grid--single': analysisState === 'absent' }"
          :aria-busy="analysisLoading"
        >
          <article class="work-card work-card--assistant">
            <div class="card-heading">
              <span class="card-icon" aria-hidden="true">
                <svg viewBox="0 0 24 24" fill="none">
                  <path d="M5.5 5.5h13v9h-7l-4 3v-3h-2v-9Z" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round" />
                  <path d="M8.5 9h7M8.5 12h4" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
                </svg>
              </span>
              <div>
                <h3>{{ loginCopy.employeeAssistant }}</h3>
                <p>{{ copy.employeeDescription }}</p>
              </div>
            </div>

            <div class="recent-work" aria-live="polite">
              <span class="recent-label">{{ copy.recentWork }}</span>
              <template v-if="employeeRecentWork">
                <RouterLink class="recent-title" :to="`/platform/chat/${employeeRecentWork.id}`">
                  {{ employeeRecentWork.title }}
                </RouterLink>
                <time v-if="employeeRecentWork.updatedAt" :datetime="employeeRecentWork.updatedAt">
                  {{ formatTime(employeeRecentWork.updatedAt) }}
                </time>
              </template>
              <span v-else class="recent-empty">{{ copy.noRecentWork }}</span>
            </div>

            <div class="card-actions">
              <RouterLink class="primary-action" to="/platform/creatChat">{{ copy.startWork }}</RouterLink>
              <RouterLink
                v-if="employeeRecentWork"
                class="text-action"
                :to="`/platform/chat/${employeeRecentWork.id}`"
              >
                {{ copy.continueWork }}
              </RouterLink>
            </div>
          </article>

          <article
            v-if="analysisState !== 'absent'"
            class="work-card work-card--analysis"
            :class="{ 'work-card--disabled': analysisState === 'disabled' }"
            :aria-disabled="analysisState === 'enabled' ? undefined : true"
          >
            <div class="card-heading">
              <span class="card-icon" aria-hidden="true">
                <svg viewBox="0 0 24 24" fill="none">
                  <path d="M5 18.5V11m4.7 7.5V6.5m4.6 12v-5m4.7 5V9" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" />
                  <path d="M4 20h16" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
                </svg>
              </span>
              <div>
                <h3>{{ loginCopy.operatingAnalysis }}</h3>
                <p>{{ copy.analysisDescription }}</p>
              </div>
            </div>

            <div class="recent-work" aria-live="polite">
              <span class="recent-label">{{ copy.recentWork }}</span>
              <template v-if="analysisState === 'enabled' && analysisRecentWork">
                <RouterLink class="recent-title" to="/platform/operating-analysis">
                  {{ analysisRecentWork.title }}
                </RouterLink>
                <time :datetime="analysisRecentWork.updatedAt">{{ formatTime(analysisRecentWork.updatedAt) }}</time>
              </template>
              <span v-else class="recent-empty">{{ analysisStatusText }}</span>
            </div>

            <div v-if="analysisState === 'enabled'" class="card-actions">
              <RouterLink class="primary-action" to="/platform/operating-analysis">{{ copy.enterWork }}</RouterLink>
            </div>
          </article>
        </div>
      </section>

      <section class="roadmap" aria-labelledby="roadmap-title">
        <div class="roadmap-copy">
          <h2 id="roadmap-title">{{ copy.roadmapTitle }}</h2>
          <p>{{ copy.roadmapDescription }}</p>
        </div>
        <ol class="roadmap-track">
          <li>
            <span>{{ copy.current }}</span>
            <strong>{{ loginCopy.employeeAssistant }}</strong>
          </li>
          <li>
            <span>{{ copy.current }}</span>
            <strong>{{ loginCopy.operatingAnalysis }}</strong>
          </li>
          <li class="roadmap-future" aria-disabled="true">
            <span>{{ copy.planned }}</span>
            <strong>{{ copy.procurementOrdering }}</strong>
          </li>
        </ol>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import UserMenu from '@/components/UserMenu.vue'
import { getSessionsList } from '@/api/chat'
import {
  getOperatingAnalysisHistory,
  type OperatingAnalysisRevocationHistoryV1,
} from '@/api/operatingAnalysis'
import {
  getProductShellLoginCopy,
  getRetailAgentHomeCopy,
  productShellBrand,
} from '@/config/productShellBrand'

type RecentWork = {
  id: string
  title: string
  updatedAt: string
}

type AnalysisState = 'absent' | 'disabled' | 'enabled'

const { locale } = useI18n()
const copy = computed(() => getRetailAgentHomeCopy(locale.value))
const loginCopy = computed(() => getProductShellLoginCopy(locale.value))

const employeeRecentWork = ref<RecentWork | null>(null)
const analysisRecentWork = ref<OperatingAnalysisRevocationHistoryV1['recentWork']>()
const analysisState = ref<AnalysisState>('absent')
const analysisLoading = ref(true)
const analysisNextAction = ref<OperatingAnalysisRevocationHistoryV1['availability']['nextAction']>('none')

const analysisStatusText = computed(() => {
  if (analysisState.value === 'enabled') return copy.value.noRecentWork
  if (analysisNextAction.value === 'contact_admin') return copy.value.contactAdmin
  return copy.value.serviceUnavailable
})

const loadEmployeeRecentWork = async () => {
  try {
    const response = await getSessionsList(1, 1, 'web') as any
    const session = response?.data?.[0]
    if (!session?.id) return
    employeeRecentWork.value = {
      id: String(session.id),
      title: String(session.title || copy.value.noRecentWork),
      updatedAt: String(session.updated_at || session.created_at || ''),
    }
  } catch {
    employeeRecentWork.value = null
  }
}

const loadOperatingAnalysis = async () => {
  try {
    const response = await getOperatingAnalysisHistory()
    analysisNextAction.value = response.availability.nextAction
    analysisState.value = response.availability.state === 'hidden'
      ? 'absent'
      : response.availability.state
    analysisRecentWork.value = response.availability.canReadHistory ? response.recentWork : undefined
  } catch (error: any) {
    analysisState.value = error?.status === 403 ? 'absent' : 'disabled'
  } finally {
    analysisLoading.value = false
  }
}

const formatTime = (value: string) => new Intl.DateTimeFormat(locale.value, {
  year: 'numeric',
  month: 'short',
  day: 'numeric',
}).format(new Date(value))

onMounted(() => {
  void loadEmployeeRecentWork()
  void loadOperatingAnalysis()
})
</script>

<style scoped lang="less">
.retail-home {
  width: 100%;
  min-height: 100%;
  overflow-x: hidden;
  overflow-y: auto;
  color: var(--product-shell-ink);
  background:
    radial-gradient(circle at 76% -10%, rgba(20, 123, 118, 0.18), transparent 34rem),
    var(--product-shell-paper);
}

.home-nav {
  position: sticky;
  top: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: min(1180px, calc(100% - 48px));
  min-height: 64px;
  margin: 0 auto;
  border-bottom: 1px solid rgba(19, 45, 45, 0.12);
  background: color-mix(in srgb, var(--product-shell-paper) 88%, transparent);
  backdrop-filter: blur(16px);
}

.brand-link {
  display: inline-flex;
  align-items: center;
  border-radius: 8px;

  img {
    display: block;
    width: 176px;
    max-width: 42vw;
    height: auto;
  }

  &:focus-visible {
    outline: 3px solid color-mix(in srgb, var(--product-shell-teal) 38%, transparent);
    outline-offset: 4px;
  }
}

.home-account {
  width: min(230px, 42vw);

  :deep(.user-dropdown) {
    top: calc(100% + 8px);
    right: 0;
    bottom: auto;
    left: auto;
    width: 260px;
    margin: 0;
  }
}

.home-main {
  width: min(1180px, calc(100% - 48px));
  margin: 0 auto;
  padding: 22px 0 72px;
}

.hero {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  animation: home-enter 360ms ease-out both;
}

.hero-eyebrow,
.section-kicker {
  margin: 0 0 12px;
  color: var(--product-shell-teal);
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.hero h1 {
  max-width: 840px;
  margin: 0;
  font-size: clamp(38px, 4vw, 52px);
  font-weight: 650;
  line-height: 1.08;
  letter-spacing: -0.04em;
  text-wrap: balance;
}

.headline-compact {
  display: none;
}

.hero-description {
  max-width: 760px;
  margin: 14px 0 0;
  color: rgba(19, 45, 45, 0.7);
  font-size: 16px;
  line-height: 1.75;
}

.operating-loop {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  margin-top: 20px;
  padding: 12px 20px;
  border: 1px solid rgba(19, 45, 45, 0.12);
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.48);
}

.loop-step {
  color: rgba(19, 45, 45, 0.8);
  font-size: 13px;
  font-weight: 600;
}

.loop-arrow {
  margin: 0 clamp(10px, 2.5vw, 30px);
  color: var(--product-shell-teal);
}

.work-section {
  margin-top: 24px;
}

.section-heading {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(300px, 0.7fr);
  align-items: end;
  gap: 40px;
  margin-bottom: 12px;

  h2 {
    margin: 0;
    font-size: clamp(27px, 2.6vw, 34px);
    line-height: 1.15;
    letter-spacing: -0.025em;
  }

  > p {
    margin: 0 0 4px;
    color: rgba(19, 45, 45, 0.68);
    line-height: 1.7;
  }
}

.work-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  grid-auto-flow: dense;
  gap: 16px;

  &--single {
    grid-template-columns: minmax(0, 1fr);

    .work-card {
      max-width: 720px;
    }
  }
}

.work-card {
  display: flex;
  min-height: 260px;
  flex-direction: column;
  padding: 24px;
  overflow: hidden;
  border: 1px solid rgba(19, 45, 45, 0.12);
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.78);
  box-shadow: 0 16px 44px rgba(19, 45, 45, 0.07);
  transition: transform 220ms ease, border-color 220ms ease, box-shadow 220ms ease;

  &:hover {
    transform: translateY(-3px);
    border-color: rgba(20, 123, 118, 0.38);
    box-shadow: 0 20px 52px rgba(19, 45, 45, 0.1);
  }

  &--analysis {
    color: #f8fbf8;
    border-color: rgba(255, 255, 255, 0.12);
    background:
      radial-gradient(circle at 90% 0%, rgba(93, 190, 177, 0.24), transparent 45%),
      #123f3e;

    .card-heading p,
    .recent-label,
    .recent-empty {
      color: rgba(248, 251, 248, 0.68);
    }

    .recent-work time {
      color: rgba(248, 251, 248, 0.68);
    }

    .recent-work {
      border-color: rgba(255, 255, 255, 0.13);
      background: rgba(255, 255, 255, 0.06);
    }

    .recent-title {
      color: #fff;
    }
  }

  &--disabled {
    background: #315453;
  }
}

.card-heading {
  display: flex;
  align-items: flex-start;
  gap: 16px;

  h3 {
    margin: 0;
    font-size: 25px;
    line-height: 1.2;
  }

  p {
    margin: 9px 0 0;
    color: rgba(19, 45, 45, 0.66);
    font-size: 14px;
    line-height: 1.65;
  }
}

.card-icon {
  display: grid;
  width: 44px;
  height: 44px;
  flex: 0 0 44px;
  place-items: center;
  border-radius: 13px;
  color: currentColor;
  background: color-mix(in srgb, currentColor 9%, transparent);

  svg {
    width: 24px;
    height: 24px;
  }
}

.recent-work {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 6px 16px;
  margin-top: 18px;
  padding: 14px 16px;
  border: 1px solid rgba(19, 45, 45, 0.1);
  border-radius: 15px;
  background: rgba(247, 242, 233, 0.65);
}

.recent-label {
  grid-column: 1 / -1;
  color: rgba(19, 45, 45, 0.52);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.recent-title {
  min-width: 0;
  overflow: hidden;
  color: var(--product-shell-ink);
  font-size: 14px;
  font-weight: 650;
  text-decoration: none;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recent-empty,
.recent-work time {
  color: rgba(19, 45, 45, 0.58);
  font-size: 13px;
}

.card-actions {
  display: flex;
  align-items: center;
  gap: 18px;
  margin-top: auto;
  padding-top: 18px;
}

.primary-action,
.text-action {
  display: inline-flex;
  min-height: 44px;
  align-items: center;
  justify-content: center;
  border-radius: 11px;
  font-size: 14px;
  font-weight: 650;
  text-decoration: none;
}

.primary-action {
  padding: 0 20px;
  border: 1px solid transparent;
  color: #fff;
  background: var(--product-shell-teal);

  &:focus-visible,
  &:hover {
    background: #0f6561;
  }

  &:focus-visible {
    outline: 3px solid rgba(116, 190, 183, 0.46);
    outline-offset: 3px;
  }
}

.work-card--analysis .primary-action {
  color: var(--product-shell-ink);
  background: #f7f2e9;

  &:hover,
  &:focus-visible {
    background: #fff;
  }
}

.text-action {
  color: var(--product-shell-teal);

  &:focus-visible {
    outline: 3px solid rgba(20, 123, 118, 0.24);
    outline-offset: 3px;
  }
}

.roadmap {
  display: grid;
  grid-template-columns: minmax(240px, 0.65fr) minmax(0, 1.35fr);
  gap: 54px;
  margin-top: 88px;
  padding: 40px 0 0;
  border-top: 1px solid rgba(19, 45, 45, 0.14);
}

.roadmap-copy {
  h2 {
    margin: 0;
    font-size: 27px;
    line-height: 1.22;
    letter-spacing: -0.02em;
  }

  p {
    margin: 14px 0 0;
    color: rgba(19, 45, 45, 0.64);
    line-height: 1.7;
  }
}

.roadmap-track {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  grid-auto-flow: dense;
  margin: 0;
  padding: 0;
  overflow: hidden;
  border: 1px solid rgba(19, 45, 45, 0.12);
  border-radius: 18px;
  list-style: none;

  li {
    display: flex;
    min-height: 112px;
    flex-direction: column;
    justify-content: space-between;
    padding: 20px;
    border-right: 1px solid rgba(19, 45, 45, 0.12);
    background: rgba(255, 255, 255, 0.5);

    &:last-child {
      border-right: 0;
    }

    span {
      color: var(--product-shell-teal);
      font-size: 12px;
      font-weight: 700;
    }

    strong {
      font-size: 17px;
    }
  }

  .roadmap-future {
    color: rgba(19, 45, 45, 0.54);
    background: rgba(19, 45, 45, 0.045);

    span {
      color: rgba(19, 45, 45, 0.44);
    }
  }
}

@keyframes home-enter {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
}

@media (max-width: 760px) {
  .home-nav,
  .home-main {
    width: min(calc(100% - 32px), 1180px);
  }

  .home-nav {
    min-height: 64px;
  }

  .home-account {
    width: 44px;

    :deep(.user-info),
    :deep(.dropdown-icon) {
      display: none;
    }

    :deep(.user-button) {
      justify-content: flex-end;
    }
  }

  .home-main {
    padding-top: 34px;
  }

  .hero {
    align-items: flex-start;
    text-align: left;
  }

  .hero h1 {
    font-size: clamp(34px, 10vw, 42px);
  }

  .headline-wide {
    display: none;
  }

  .headline-compact {
    display: inline;
  }

  .hero-description {
    font-size: 15px;
  }

  .operating-loop {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    align-items: stretch;
    padding: 16px;
    gap: 12px;
  }

  .loop-step {
    min-width: 0;
  }

  .loop-arrow {
    display: none;
  }

  .section-heading,
  .roadmap {
    grid-template-columns: 1fr;
    gap: 18px;
  }

  .section-heading > p {
    display: none;
  }

  .work-section {
    margin-top: 38px;
  }

  .work-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }

  .work-grid--single {
    grid-template-columns: 1fr;
  }

  .work-card {
    min-height: 250px;
    padding: 16px;
    border-radius: 18px;
  }

  .card-heading {
    display: block;

    h3 {
      margin-top: 12px;
      font-size: 21px;
    }

    p {
      display: none;
    }
  }

  .card-icon {
    width: 38px;
    height: 38px;
    flex-basis: 38px;
  }

  .recent-work {
    grid-template-columns: 1fr;
    min-height: 78px;
    margin-top: 14px;
    padding: 12px;
  }

  .recent-title {
    display: -webkit-box;
    overflow: hidden;
    white-space: normal;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 2;
  }

  .recent-work time {
    display: none;
  }

  .card-actions {
    display: block;
    padding-top: 14px;
  }

  .primary-action {
    width: 100%;
    min-height: 40px;
    padding: 8px 10px;
    text-align: center;
  }

  .text-action {
    display: none;
  }

  .roadmap {
    margin-top: 64px;
  }

  .roadmap-track {
    grid-template-columns: 1fr;

    li {
      min-height: 92px;
      border-right: 0;
      border-bottom: 1px solid rgba(19, 45, 45, 0.12);

      &:last-child {
        border-bottom: 0;
      }
    }
  }
}

@media (prefers-reduced-motion: reduce) {
  .hero,
  .work-card,
  .primary-action,
  .text-action {
    animation: none;
    transition: none;
  }
}
</style>
