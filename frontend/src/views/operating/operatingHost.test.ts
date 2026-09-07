import assert from 'node:assert/strict'
import test from 'node:test'
import {
  normalizedOperatingRoute,
  operatingLocationFromRoute,
  operatingLocationKey,
  operatingNavigationStateFromRoute,
  operatingRouteIsCanonical,
  operatingRouteLocation,
  operatingRouteLocationFromRuntime,
  parseOperatingNavigationState,
  runtimeNavigationTarget,
  shouldRestoreOperatingLocation,
} from './operatingHost.ts'

test('deep-link state round-trips between the Vue route and runtime contract', () => {
  const location = operatingLocationFromRoute('/platform/operating-analysis', {
    data_surface: 'brief',
    data_session: 'session-1',
    data_mode: 'deep',
    data_artifact: 'artifact-1',
    data_tab: 'chart',
  })

  assert.deepEqual(location, {
    surface: 'analysis',
    sessionId: 'session-1',
    mode: 'deep',
    artifactId: 'artifact-1',
    tab: 'chart',
  })
  assert.deepEqual(operatingRouteLocationFromRuntime(location), {
    path: '/platform/operating-analysis',
    query: {
      data_surface: 'analysis',
      data_session: 'session-1',
      data_mode: 'deep',
      data_artifact: 'artifact-1',
      data_tab: 'chart',
    },
  })
})

test('missing route fields receive the runtime contract defaults', () => {
  assert.deepEqual(operatingLocationFromRoute('/platform/operating-brief', {}), {
    surface: 'brief',
    sessionId: null,
    artifactId: null,
    mode: 'auto',
    tab: 'report',
  })
})

test('route normalization allowlists known keys and makes the path authoritative for surface', () => {
  assert.deepEqual(normalizedOperatingRoute('/platform/operating-brief', {
    data_surface: 'analysis',
    data_session: 'https://foreign.test/session',
    data_mode: 'quick',
    data_artifact: ['duplicate', 'artifact'],
    data_workspace: 'category',
    redirect: '/login',
  }), {
    path: '/platform/operating-brief',
    query: { data_surface: 'brief', data_mode: 'quick' },
  })
  assert.equal(operatingRouteIsCanonical('/platform/operating-brief', {
    data_surface: 'brief',
    data_mode: 'quick',
  }), true)
  assert.equal(operatingRouteIsCanonical('/platform/operating-brief', {
    data_surface: 'brief',
    redirect: '/login',
  }), false)
})

test('runtime navigation rejects invalid IDs, ignores equivalent routes and cannot steer while inactive', () => {
  const currentQuery = {
    data_surface: 'analysis',
    data_session: 'session-2',
    data_mode: 'quick',
    data_tab: 'report',
  }
  const next = {
    surface: 'analysis' as const,
    sessionId: 'session-2',
    artifactId: null,
    mode: 'quick' as const,
    tab: 'report' as const,
  }

  assert.equal(runtimeNavigationTarget(false, '/platform/creatChat', {}, next), null)
  assert.equal(runtimeNavigationTarget(true, '/platform/operating-analysis', currentQuery, next), null)
  assert.equal(operatingRouteLocationFromRuntime({
    ...next,
    sessionId: 'https://foreign.test/session',
  }), null)
  assert.deepEqual(runtimeNavigationTarget(true, '/platform/operating-analysis', currentQuery, {
    ...next,
    sessionId: 'session-3',
  }), {
    path: '/platform/operating-analysis',
    query: { ...currentQuery, data_session: 'session-3' },
  })
})

test('feedback keys are stable and session IDs come from route fields only', () => {
  const state = operatingNavigationStateFromRoute('/platform/operating-analysis', {
    data_session: 'session-4',
    question: 'open session-evil',
  })
  assert.deepEqual(state, { data_surface: 'analysis', data_session: 'session-4' })
  assert.equal(operatingLocationKey(operatingLocationFromRoute('/platform/operating-analysis', state)),
    'analysis\0session-4\0\0auto\0report')
  assert.deepEqual(operatingRouteLocation(state), {
    path: '/platform/operating-analysis',
    query: state,
  })
  assert.equal(parseOperatingNavigationState({
    data_surface: 'analysis',
    data_session: 'session-4',
    source_ref: 'private-source',
  }), null)
})

test('plain menu re-entry restores snapshot context but an explicit handoff starts independently', () => {
  const requested = { data_surface: 'analysis' as const }
  const previous = {
    surface: 'analysis' as const,
    sessionId: 'session-5',
    artifactId: 'artifact-2',
    mode: 'deep' as const,
    tab: 'report' as const,
  }
  assert.equal(shouldRestoreOperatingLocation(true, false, requested, previous), true)
  assert.equal(shouldRestoreOperatingLocation(true, true, requested, previous), false)
  assert.equal(shouldRestoreOperatingLocation(false, false, requested, previous), false)
  assert.equal(shouldRestoreOperatingLocation(true, false, {
    ...requested,
    data_session: 'explicit-session',
  }, previous), false)
})
