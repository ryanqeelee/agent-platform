import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

const menuStoreSource = readFileSync(fileURLToPath(new URL('../stores/menu.ts', import.meta.url)), 'utf8')
const userMessageSource = readFileSync(fileURLToPath(new URL('../views/chat/components/usermsg.vue', import.meta.url)), 'utf8')

test('employee assistance and operating analysis use distinct navigation icons', () => {
  assert.match(menuStoreSource, /titleKey: 'menu\.newChat',[\s\S]*?icon: 'assistant'/)
  assert.match(menuStoreSource, /titleKey: 'menu\.operatingAnalysis', icon: 'analysis'/)
})

test('the operating-analysis handoff remains discoverable without hover', () => {
  assert.match(userMessageSource, /menu\.continueInOperatingAnalysis/)
  assert.match(userMessageSource, /analysis-green\.svg/)
  assert.doesNotMatch(userMessageSource, /\.user_msg_container:hover \.analysis_handoff/)
})
