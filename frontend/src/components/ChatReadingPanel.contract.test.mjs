import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const panel = readFileSync(new URL('./ChatReadingPanel.vue', import.meta.url), 'utf8')
const references = readFileSync(new URL('./ChatReferencesDrawer.vue', import.meta.url), 'utf8')

test('employee references use the shared panel with the existing default shell', () => {
  assert.match(references, /<ChatReadingPanel[\s\S]*?:visible="visible"[\s\S]*?:title="panelTitle"/)
  assert.match(panel, /overlayBreakpoint: 960/)
  assert.match(panel, /variant: 'references'/)
  assert.match(panel, /width: min\(420px, 100vw\)/)
  assert.doesNotMatch(references, /\.chat-references-panel\s*\{/)
})

test('reading variant supports wide, full-screen and narrow overlay layouts', () => {
  assert.match(panel, /'is-reading': variant === 'reading'/)
  assert.match(panel, /'is-expanded': expanded/)
  assert.match(panel, /&\.is-expanded\s*\{\s*width: calc\(100vw - 72px\)/)
  assert.match(panel, /@media \(max-width: 959px\)[\s\S]*?width: 100vw/)
  assert.match(panel, /<Teleport to="body" :disabled="!useOverlay">/)
  assert.match(panel, /window\.addEventListener\('resize', updateViewportWidth\)/)
  assert.match(panel, /window\.removeEventListener\('resize', updateViewportWidth\)/)
})

test('reading panel exposes keyboard close, focus and a constrained scroll body', () => {
  assert.match(panel, /@keydown\.esc="handleEscape"/)
  assert.match(panel, /if \(props\.focusOnOpen\) closeButton\.value\?\.focus\(\)/)
  assert.match(panel, /if \(props\.closeOnEscape\) emit\('close'\)/)
  assert.match(panel, /\.chat-references-panel__body\s*\{[\s\S]*?min-width: 0;[\s\S]*?overflow: auto;/)
})
