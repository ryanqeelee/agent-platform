import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'
import { createMemoryHistory, createRouter } from 'vue-router'
import {
  DEFAULT_EMPLOYEE_WORKSPACE_PATH,
  defaultAuthenticatedDestination,
  oidcInvitationDestination,
  PLATFORM_OPERATIONS_PATH,
  postLoginDestination,
  safeReturnTo,
  tenantRequiredRouteFallback,
  WORKSPACE_ONBOARDING_PATH,
} from './safeReturnTo'

const router = createRouter({
  history: createMemoryHistory(),
  routes: [
    { path: '/login', component: { template: '<div />' } },
    { path: '/register', component: { template: '<div />' } },
    { path: '/onboarding/workspace', component: { template: '<div />' } },
    { path: '/platform/creatChat', component: { template: '<div />' } },
    { path: '/platform/knowledge-bases', component: { template: '<div />' } },
    { path: '/platform/enterprise', component: { template: '<div />' } },
    { path: PLATFORM_OPERATIONS_PATH, component: { template: '<div />' } },
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
    postLoginDestination(router, '/platform/enterprise?section=members', true, false),
    '/platform/enterprise?section=members',
  )
})

test('postLoginDestination falls back after an invalid OIDC return target', () => {
  assert.equal(postLoginDestination(router, 'https://evil.example', true, false), DEFAULT_EMPLOYEE_WORKSPACE_PATH)
  assert.equal(DEFAULT_EMPLOYEE_WORKSPACE_PATH, '/home')
})

test('system administrators default to the tenant-independent operations page', () => {
  assert.equal(defaultAuthenticatedDestination(false, true), PLATFORM_OPERATIONS_PATH)
  assert.equal(defaultAuthenticatedDestination(true, true), PLATFORM_OPERATIONS_PATH)
  assert.equal(postLoginDestination(router, undefined, false, true), PLATFORM_OPERATIONS_PATH)
  assert.equal(
    postLoginDestination(router, '/platform/enterprise?section=members', true, true),
    PLATFORM_OPERATIONS_PATH,
  )
})

test('OIDC invitation acceptance keeps the enterprise landing while routing platform identities to operations', () => {
  assert.equal(oidcInvitationDestination(false), '/platform/knowledge-bases')
  assert.equal(oidcInvitationDestination(true), PLATFORM_OPERATIONS_PATH)
  const app = readFileSync(new URL('../App.vue', import.meta.url), 'utf8')
  assert.match(app, /router\.replace\(oidcInvitationDestination\(authStore\.isSystemAdmin\)\)/)
})

test('tenant routes always send a platform identity to operations', () => {
  assert.equal(tenantRequiredRouteFallback(false, true), PLATFORM_OPERATIONS_PATH)
  assert.equal(tenantRequiredRouteFallback(true, true), PLATFORM_OPERATIONS_PATH)
  assert.equal(tenantRequiredRouteFallback(false, false), WORKSPACE_ONBOARDING_PATH)
  assert.equal(tenantRequiredRouteFallback(true, false), null)
})

test('platform operations is a top-level route outside the enterprise platform shell', () => {
  const source = readFileSync(new URL('./index.ts', import.meta.url), 'utf8')
  const operationsRoute = source.indexOf('path: PLATFORM_OPERATIONS_PATH')
  const platformShell = source.indexOf('path: "/platform"')

  assert.ok(operationsRoute > 0 && operationsRoute < platformShell)
  assert.match(source, /path: PLATFORM_OPERATIONS_PATH,[\s\S]*?meta: \{ requiresAuth: true, requiresTenant: false, requiresSystemAdmin: true \}/)
  assert.doesNotMatch(source, /path: "operations"/)
})

test('password, OIDC, and existing-session login flows use the platform-aware destination', () => {
  const login = readFileSync(new URL('../views/auth/Login.vue', import.meta.url), 'utf8')
  const app = readFileSync(new URL('../App.vue', import.meta.url), 'utf8')
  const routeGuard = readFileSync(new URL('./index.ts', import.meta.url), 'utf8')
  const destinationCall = /postLoginDestination\(router, returnTo, authStore\.hasValidTenant, authStore\.isSystemAdmin\)/g

  assert.equal(login.match(destinationCall)?.length, 2)
  assert.equal(app.match(destinationCall)?.length, 1)
  assert.equal(routeGuard.match(destinationCall)?.length, 1)
})
