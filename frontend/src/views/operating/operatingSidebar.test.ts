import assert from 'node:assert/strict'
import test from 'node:test'
import { operatingSessionLocation, projectOperatingSidebar } from './operatingSidebar.ts'

const location = {
  surface: 'analysis' as const,
  sessionId: 'session-1',
  artifactId: null,
  mode: 'auto' as const,
  tab: 'report' as const,
}

test('projects Center sessions into native sidebar rows and marks the active session', () => {
  assert.deepEqual(projectOperatingSidebar({
    location,
    sessions: [
      { id: 'session-1', title: '门店趋势', updatedAt: '2026-09-07T08:00:00Z' },
      { id: 'session-2', title: '', updatedAt: '2026-09-06T08:00:00Z' },
    ],
  }), {
    activePath: 'session-1',
    sessions: [
      { id: 'session-1', path: 'session-1', title: '门店趋势', updated_at: '2026-09-07T08:00:00Z' },
      { id: 'session-2', path: 'session-2', title: '未命名分析', updated_at: '2026-09-06T08:00:00Z' },
    ],
  })
})

test('new drafts retain saved history, snapshot additions appear, and switching changes only the active row', () => {
  assert.deepEqual(projectOperatingSidebar(null), { activePath: '', sessions: [] })

  const saved = [{ id: 'session-1', title: '门店趋势', updatedAt: '2026-09-07T08:00:00Z' }]
  const draft = projectOperatingSidebar({
    location: { ...location, sessionId: null },
    sessions: saved,
  })
  assert.equal(draft.activePath, '')
  assert.deepEqual(draft.sessions.map((session) => session.id), ['session-1'])

  const refreshed = projectOperatingSidebar({
    location: { ...location, sessionId: 'session-2' },
    sessions: [
      ...saved,
      { id: 'session-2', title: '品类毛利', updatedAt: '2026-09-07T09:00:00Z' },
    ],
  })
  assert.equal(refreshed.activePath, 'session-2')
  assert.deepEqual(refreshed.sessions.map((session) => session.id), ['session-1', 'session-2'])
})


test('history selection opens analysis and preserves reading only for the same conversation', () => {
  for (const surface of ['brief', 'analysis'] as const) {
    const current = { ...location, surface, artifactId: 'report-1', tab: 'chart' as const }
    assert.deepEqual(operatingSessionLocation(current, 'session-1'), { ...current, surface: 'analysis' })
    assert.deepEqual(operatingSessionLocation(current, 'session-2'), {
      ...current, surface: 'analysis', sessionId: 'session-2', artifactId: null, tab: 'report',
    })
  }
})
