import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import test from 'node:test'

const source = readFileSync(new URL('./platformOperations.ts', import.meta.url), 'utf8')
const viewSource = readFileSync(new URL('../views/operations/PlatformOperations.vue', import.meta.url), 'utf8')

test('platform operations uses the frozen enterprise mutation and activation contracts', () => {
  assert.match(source, /patch<OperationsResponse<OperationsEnterprise>>\(`\$\{base\}\/enterprises\/\$\{tenantId\}`/)
  assert.match(source, /productBaseTenantId: string \| null/)
  assert.match(source, /'Idempotency-Key': idempotencyKey/)
  assert.match(source, /initial_administrator_user_id: string/)
  assert.match(viewSource, /const payload = enterpriseUpdatePayload\(editEnterprise\)/)
  assert.match(viewSource, /updateOperationsEnterprise\(selected\.value\.id, payload\)/)
})

test('initial administrator creation has a stable command and a recovery read', () => {
  assert.match(source, /post<OperationsResponse<InitialAdministratorCommandResponse>>\([\s\S]*?'Idempotency-Key': commandId/)
  assert.match(source, /initial-administrators\/\$\{encodeURIComponent\(commandId\)\}/)
  assert.doesNotMatch(source, /generated_password/)
})

test('member listing sends search and explicit pagination instead of a fixed first page', () => {
  assert.match(source, /page: String\(page\), page_size: String\(pageSize\)/)
  assert.match(source, /params\.set\('q', query\.trim\(\)\)/)
  assert.doesNotMatch(source, /page_size=100/)
})

test('receipt recovery loses password knowledge and non-active enterprises block member mutations', () => {
  assert.match(viewSource, /admin = await getInitialAdministratorCommand\(command\.commandId\)\s+initialAdminPasswordUnknown\.value = true/)
  assert.match(viewSource, /selected\.value\?\.status !== 'active'/)
  assert.match(viewSource, /企业已暂停，成员新增、角色、状态和密码操作不可用/)
  assert.match(viewSource, /:disabled="enterpriseMemberActionsDisabled"/)
})

test('an abandoned activation has an explicit browser-only restart action', () => {
  assert.match(viewSource, /activationStatus === 'abandoned'.*关闭此失败命令并开始新的企业创建/)
  const restart = viewSource.match(/function restartAfterAbandonedActivation\(\) \{[\s\S]*?\n\}/)?.[0] || ''
  assert.match(restart, /sessionStorage\.removeItem\(PENDING_ACTIVATION_STORAGE_KEY\)/)
  assert.match(restart, /sessionStorage\.removeItem\(PENDING_INITIAL_ADMIN_STORAGE_KEY\)/)
  assert.match(restart, /pendingActivationCommand\.value = null/)
  assert.match(restart, /pendingInitialAdminCommand\.value = null/)
  assert.match(restart, /initialAdminUserID\.value = ''/)
  assert.match(restart, /initialAdminPasswordUnknown\.value = false/)
  assert.match(restart, /Object\.assign\(creation, freshEnterpriseCreationDraft\(\)\)/)
  assert.doesNotMatch(restart, /delete|removeInitialAdministrator/)
})
