import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./AgentStreamDisplay.vue', import.meta.url), 'utf8')

test('process-only mode exposes the operating timeline contract', () => {
  assert.match(source, /processOnly\?: boolean/)
  assert.match(source, /operatingStatus\?: OperatingStatus/)
  assert.match(source, /operatingDurationMs\?: number \| null/)
  assert.match(source, /return isOperatingTerminalStatus\(props\.operatingStatus\)/)
})

test('process-only mode allowlists timeline events and never projects an answer fallback', () => {
  assert.match(source, /const OPERATING_PROCESS_EVENT_TYPES = new Set\(\[/)
  for (const type of ['thinking', 'tool_call', 'plan_task_change', 'context_compacted']) {
    assert.match(source, new RegExp(`'${type}'`))
  }
  assert.match(source, /if \(!props\.processOnly\) return stream;[\s\S]*?\.filter\(\(event: any\) => event && OPERATING_PROCESS_EVENT_TYPES\.has\(event\.type\)\)/)
  assert.match(source, /terminal && event\.pending \? \{ \.\.\.event, pending: false \} : event/)
  assert.match(source, /const finalContent = computed\(\(\) => \{\s*if \(props\.processOnly\) return null;/)
  assert.match(source, /if \(props\.processOnly\) \{\s*return isConversationDone\.value \? \[\] : result;/)
})

test('process-only mode gates employee reference, memory, artifact and auth surfaces', () => {
  assert.match(source, /v-if="!processOnly && hasMemory"/)
  assert.match(source, /<ChatCitationFloat v-if="!processOnly"/)
  assert.match(source, /v-if="!processOnly && hasArtifacts/)
  assert.match(source, /!props\.processOnly && getToolReferenceItems\(event\)/)
  assert.match(source, /useChatCitationPopover\(props\.processOnly \? ref<HTMLElement \| null>\(null\) : rootElement/)
  assert.match(source, /if \(props\.processOnly\) return;\s*nextTick\(async \(\) => \{\s*const root = rootElement\.value;/)
})

test('process-only duration comes only from the optional host value', () => {
  assert.match(source, /props\.processOnly\s*\? \(props\.operatingDurationMs \?\? 0\)\s*: employeeAgentDurationMs\.value/)
  assert.match(source, /if \(props\.processOnly\) return;\s*if \(!stream \|\| !Array\.isArray\(stream\)\) return;/)
})

test('only an operating query with real projected detail emits its stable result identity', () => {
  assert.match(source, /event\?\.tool_data\?\.result_available === true/)
  assert.match(source, /emit\(\s*'operating-result',\s*props\.operatingMessageId,\s*String\(event\.tool_call_id\)\.replace\(\/\^operating-query-\//)
  assert.match(source, /if \(props\.processOnly && event\?\.tool_name === 'database_query'\) return canOpenOperatingResult\(event\);/)
  assert.match(source, /if \(props\.processOnly && event\?\.tool_name === 'data_analysis'\) return false;/)
})
