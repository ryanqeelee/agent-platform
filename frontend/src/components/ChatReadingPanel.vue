<template>
  <Teleport to="body" :disabled="!useOverlay">
    <Transition name="references-panel" @after-enter="handleAfterEnter">
      <aside
        v-if="visible || keepMounted"
        v-show="visible"
        class="chat-references-panel"
        :class="{
          'is-overlay': useOverlay,
          'is-embedded': embeddedMode,
          'is-reading': variant === 'reading',
          'is-expanded': expanded,
        }"
        role="complementary"
        :aria-label="title"
        @keydown.esc="handleEscape"
      >
        <header class="chat-references-panel__header">
          <div class="chat-references-panel__heading">
            <h3 class="chat-references-panel__title">
              {{ title }}<span v-if="count" class="chat-references-panel__count"> · {{ count }}</span>
            </h3>
          </div>
          <div class="chat-references-panel__actions">
            <slot name="actions" />
            <button
              v-if="expandable"
              type="button"
              class="chat-references-panel__close"
              :aria-label="expanded ? '退出全屏阅读' : '全屏阅读'"
              @click="expanded = !expanded"
            >
              <t-icon :name="expanded ? 'fullscreen-exit' : 'fullscreen'" size="20px" />
            </button>
            <button
              ref="closeButton"
              type="button"
              class="chat-references-panel__close"
              :aria-label="closeLabel"
              @click="emit('close')"
            >
              <t-icon name="close" size="20px" />
            </button>
          </div>
        </header>

        <div ref="bodyElement" class="chat-references-panel__body">
          <slot />
        </div>
      </aside>
    </Transition>
  </Teleport>

  <Transition name="references-backdrop">
    <div
      v-if="visible && useOverlay"
      class="chat-references-panel__backdrop"
      @click="emit('close')"
    />
  </Transition>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  visible: boolean
  title: string
  count?: number
  closeLabel: string
  embeddedMode?: boolean
  overlayBreakpoint?: number
  variant?: 'references' | 'reading'
  expandable?: boolean
  focusOnOpen?: boolean
  closeOnEscape?: boolean
  keepMounted?: boolean
}>(), {
  count: 0,
  embeddedMode: false,
  overlayBreakpoint: 960,
  variant: 'references',
  expandable: false,
  focusOnOpen: false,
  closeOnEscape: false,
  keepMounted: false,
})

const emit = defineEmits<{
  close: []
  afterEnter: [body: HTMLElement]
}>()

const bodyElement = ref<HTMLElement | null>(null)
const closeButton = ref<HTMLButtonElement | null>(null)
const expanded = ref(false)
const viewportWidth = ref(typeof window === 'undefined' ? 960 : window.innerWidth)

const useOverlay = computed(() => {
  if (props.embeddedMode) return true
  return viewportWidth.value < props.overlayBreakpoint
})

function updateViewportWidth() {
  viewportWidth.value = window.innerWidth
}

async function handleAfterEnter() {
  await nextTick()
  if (props.focusOnOpen) closeButton.value?.focus()
  if (bodyElement.value) emit('afterEnter', bodyElement.value)
}

function handleEscape() {
  if (props.closeOnEscape) emit('close')
}

watch(() => props.visible, (visible) => {
  if (!visible) expanded.value = false
})

onMounted(() => window.addEventListener('resize', updateViewportWidth))
onBeforeUnmount(() => window.removeEventListener('resize', updateViewportWidth))
</script>

<style scoped lang="less">
.chat-references-panel__backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.28);
  z-index: 1200;
}

.chat-references-panel {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  width: min(420px, 100vw);
  z-index: 1201;
  display: flex;
  flex-direction: column;
  background: var(--td-bg-color-container);
  border-left: 1px solid var(--td-component-stroke);
  box-shadow: -8px 0 24px rgba(0, 0, 0, 0.06);

  &.is-overlay {
    box-shadow: -12px 0 32px rgba(0, 0, 0, 0.12);
  }

  &.is-reading {
    width: var(--chat-reading-width, min(760px, 100vw));
  }

  &.is-expanded {
    width: calc(100vw - 72px);
  }
}

.chat-references-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 16px 16px 12px;
  border-bottom: 1px solid var(--td-component-stroke);
}

.chat-references-panel__heading {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.chat-references-panel__title {
  margin: 0;
  font-size: 14px;
  font-weight: 500;
  color: var(--td-text-color-secondary);
  line-height: 1.4;
}

.chat-references-panel__count {
  color: var(--td-text-color-placeholder);
  font-weight: 500;
}

.chat-references-panel__actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.chat-references-panel__close {
  border: 0;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-secondary);
  width: 36px;
  height: 36px;
  border-radius: 10px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  transition: background 0.15s ease, color 0.15s ease;

  :deep(.t-icon) {
    font-size: 20px;
  }

  &:hover {
    background: color-mix(in srgb, var(--td-text-color-primary) 8%, var(--td-bg-color-secondarycontainer));
    color: var(--td-text-color-primary);
  }
}

.chat-references-panel__body {
  flex: 1;
  min-width: 0;
  overflow: auto;
  padding: 4px 12px 24px;
}

.references-panel-enter-active {
  transition:
    transform 0.24s cubic-bezier(0.22, 0.61, 0.36, 1),
    opacity 0.24s cubic-bezier(0.22, 0.61, 0.36, 1);
}

.references-panel-leave-active {
  transition:
    transform 0.3s cubic-bezier(0.22, 0.61, 0.36, 1),
    opacity 0.3s cubic-bezier(0.22, 0.61, 0.36, 1);
}

.references-panel-enter-from,
.references-panel-leave-to {
  transform: translateX(100%);
  opacity: 0.6;
}

// Reading panels already reserve their desktop column. Translating a fixed panel beyond
// the viewport briefly widens the document and shows a horizontal scrollbar during opening.
.is-reading.references-panel-enter-active,
.is-reading.references-panel-leave-active {
  transition: opacity 0.18s ease;
}

.is-reading.references-panel-enter-from,
.is-reading.references-panel-leave-to {
  transform: none;
  opacity: 0;
}

.references-backdrop-enter-active {
  transition: opacity 0.24s ease;
}

.references-backdrop-leave-active {
  transition: opacity 0.3s ease;
}

.references-backdrop-enter-from,
.references-backdrop-leave-to {
  opacity: 0;
}

@media (max-width: 959px) {
  .chat-references-panel.is-reading,
  .chat-references-panel.is-expanded {
    width: 100vw;
  }
}
</style>
