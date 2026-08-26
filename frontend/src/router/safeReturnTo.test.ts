import assert from 'node:assert/strict'
import test from 'node:test'
import { createMemoryHistory, createRouter } from 'vue-router'
import { DEFAULT_EMPLOYEE_WORKSPACE_PATH, postLoginDestination, safeReturnTo } from './safeReturnTo'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/login', component: { template: '<div />' } },
    { path: '/register', component: { template: '<div />' } },
    { path: '/onboarding/workspace', component: { template: '<div />' } },
    { path: '/platform/knowledge-bases', component: { template: '<div />' } },
    { path: '/platform/enterprise', component: { template: '<div />' } },
  ],
})

test('safeReturnTo keeps known employee deep links', () => {
  assert.equal(safeReturnTo(router, '/platform/enterprise?section=members'), '/platform/enterprise?section=members')
})

for (const target of ['//evil.example', 'https://evil.example', '/platform\\evil', '/platform/not-a-route']) {
  test(`safeReturnTo rejects ${target}`, () => {
    assert.equal(safeReturnTo(router, target), null)
  })
}

test('postLoginDestination restores a verified OIDC enterprise deep link', () => {
  assert.equal(
    postLoginDestination(router, '/platform/enterprise?section=members', true),
    '/platform/enterprise?section=members',
  )
})

test('postLoginDestination falls back after an invalid OIDC return target', () => {
  assert.equal(postLoginDestination(router, 'https://evil.example', true), DEFAULT_EMPLOYEE_WORKSPACE_PATH)
})
