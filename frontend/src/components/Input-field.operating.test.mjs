import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const inputField = readFileSync(new URL('./Input-field.vue', import.meta.url), 'utf8')
const attachmentUpload = readFileSync(new URL('./AttachmentUpload.vue', import.meta.url), 'utf8')

test('operating composer is controlled through the native input surface', () => {
  assert.match(inputField, /operating\?: OperatingComposer/)
  assert.match(inputField, /v-model="composerDraft"/)
  assert.match(inputField, /props\.operating\.setDraft\(text\)/)
  assert.match(inputField, /sendOperatingDraft\(props\.operating, val\)/)
  assert.match(inputField, /:operating="operating"/)
  assert.match(inputField, /if \(event\.e\.keyCode === 13 && !event\.e\.shiftKey/)
})

test('operating callbacks do not clear or auto-send the controlled draft', () => {
  const operatingSend = inputField.slice(
    inputField.indexOf('if (props.operating) {\n    sendOperatingDraft'),
    inputField.indexOf("if (!val.trim())", inputField.indexOf('const createSession')),
  )
  assert.doesNotMatch(operatingSend, /clearvalue|attachmentUploadRef\.value\?\.clear/)

  const controlledUpload = attachmentUpload.slice(
    attachmentUpload.indexOf('if (props.operating) {', attachmentUpload.indexOf('const addFiles')),
    attachmentUpload.indexOf('for (const file of files)', attachmentUpload.indexOf('const addFiles')),
  )
  assert.match(controlledUpload, /props\.operating\.upload\(acceptedFiles\)/)
  assert.doesNotMatch(controlledUpload, /send/)
})

test('operating mode bypasses employee resource and temporary attachment lifecycles', () => {
  const inputMount = inputField.slice(
    inputField.indexOf('onMounted(() => {'),
    inputField.indexOf('onUnmounted(() => {'),
  )
  assert.match(inputMount, /if \(props\.operating\) \{[\s\S]*?return;\s*\}/)
  assert.ok(inputMount.indexOf('return;') < inputMount.indexOf('loadKnowledgeBases()'))

  const attachmentMount = attachmentUpload.slice(
    attachmentUpload.indexOf('onMounted(async () => {'),
    attachmentUpload.indexOf('const maxFiles'),
  )
  assert.match(attachmentMount, /if \(props\.operating \|\| !authStore\?\.isSystemAdmin\) return;/)
  assert.match(attachmentUpload, /props\.operating \? \['\.csv', '\.xlsx'\]/)
  assert.match(attachmentUpload, /if \(props\.operating\) \{\s*props\.operating\.remove\(id\);\s*return;/)
})

test('operating stop delegates only to the controlled owner', () => {
  const stopBranch = inputField.slice(
    inputField.indexOf('if (props.operating) {', inputField.indexOf('const handleStop')),
    inputField.indexOf('if (!props.sessionId)', inputField.indexOf('const handleStop')),
  )
  assert.match(stopBranch, /props\.operating\.stop\(\)/)
  assert.doesNotMatch(stopBranch, /stopSession|emit\('stop-generation'\)/)
})
