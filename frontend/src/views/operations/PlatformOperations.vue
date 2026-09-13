<template>
  <main class="operations-page">
    <header class="page-header">
      <div>
        <p class="eyebrow">平台运营</p>
        <h1>企业、成员与节点管理</h1>
        <p>创建企业，维护企业成员、配额和边缘节点接入。</p>
      </div>
      <div class="page-actions">
        <t-button variant="outline" @click="uiStore.openSettings('models')">模型配置</t-button>
        <t-button variant="outline" :disabled="!selected" @click="openTenantSettings('sandbox')">沙箱配置</t-button>
        <t-button variant="outline" :disabled="!selected" @click="openTenantSettings('skills')">技能管理</t-button>
        <t-button variant="outline" :disabled="!selected" @click="openTenantSettings('chathistory')">消息索引</t-button>
        <t-button variant="outline" :disabled="!selected" @click="openTenantSettings('memory-runtime')">记忆运行配置</t-button>
        <t-button theme="primary" @click="openCreationWizard">创建企业</t-button>
        <small v-if="!selected" class="tenant-settings-hint">选择企业后可配置沙箱、技能和消息索引</small>
      </div>
    </header>

    <t-alert v-if="errorMessage" theme="error" :message="errorMessage" close @close="errorMessage = ''" />
    <t-alert v-if="passwordResetNotice" theme="warning" :message="passwordResetNotice" close @close="passwordResetNotice = ''" />

    <section class="workspace">
      <t-card class="enterprise-list" title="企业">
        <t-loading :loading="loadingEnterprises">
          <button
            v-for="enterprise in enterprises"
            :key="enterprise.id"
            class="enterprise-row"
            :class="{ active: selected?.id === enterprise.id }"
            type="button"
            @click="selectEnterprise(enterprise)"
          >
            <span><strong>{{ enterprise.name }}</strong><small>#{{ enterprise.id }}</small></span>
            <span class="enterprise-row__summary">
              <t-tag size="small" :theme="enterpriseStatusTheme(enterprise.status)">
                {{ enterpriseStatusLabel(enterprise.status) }}
              </t-tag>
              <small>{{ enterprise.seats_used }} / {{ enterprise.seats_total ?? '∞' }}</small>
            </span>
          </button>
          <t-empty v-if="!loadingEnterprises && enterprises.length === 0" description="暂无企业" />
        </t-loading>
      </t-card>

      <div v-if="selected" class="detail-column">
        <t-card title="企业信息">
          <t-form label-align="top" @submit="saveEnterprise">
            <t-form-item label="企业名称"><t-input v-model="editEnterprise.name" /></t-form-item>
            <t-form-item label="说明"><t-textarea v-model="editEnterprise.description" /></t-form-item>
            <div class="form-grid">
              <t-form-item label="企业状态">
                <t-select v-model="editEnterprise.status">
                  <t-option value="active" label="启用" />
                  <t-option value="suspended" label="暂停" />
                </t-select>
              </t-form-item>
              <t-form-item label="席位总数">
                <t-input-number v-model="editEnterprise.seats_total" :min="Math.max(1, selected.seats_used)" placeholder="留空表示不限额" />
                <small class="hint">已用 {{ selected.seats_used }} 个席位；留空保持不限额。</small>
              </t-form-item>
              <t-form-item label="存储配额（GiB）">
                <t-input-number v-model="editEnterprise.storage_quota_gib" :min="0" />
                <small class="hint">0 表示不限额。</small>
                <small v-if="selected.storage_used !== undefined" class="hint">已用 {{ formatStorage(selected.storage_used) }}</small>
              </t-form-item>
            </div>
            <t-button type="submit" :loading="savingEnterprise">保存企业信息</t-button>
          </t-form>
        </t-card>

        <t-card title="节点管理">
          <template #actions>
            <div class="actions">
              <t-button variant="outline" size="small" :loading="loadingEdge" @click="loadEnterpriseEdge">刷新</t-button>
              <t-button theme="primary" size="small" :loading="rotatingEnrollmentToken" @click="rotateEnrollmentToken">
                {{ enterpriseEdge?.summary.nodeCount ? '轮换接入令牌' : '生成接入令牌' }}
              </t-button>
            </div>
          </template>
          <t-alert v-if="edgeErrorMessage" theme="warning" :message="edgeErrorMessage" />
          <t-alert
            v-if="enrollmentToken"
            theme="warning"
            message="接入令牌仅显示这一次，请立即复制并妥善保存。再次轮换后旧令牌失效。"
            close
            @close="enrollmentToken = ''"
          />
          <div v-if="enrollmentToken" class="secret-row">
            <div>
              <small>一次性令牌</small>
              <pre class="secret">{{ enrollmentToken }}</pre>
            </div>
            <div>
              <small>边缘节点 edge.env</small>
              <pre class="secret">{{ edgeEnrollmentConfig }}</pre>
              <small class="hint">保存为 /data/edge-agent/env/edge.env 后执行 register_edge_node.sh 完成注册。</small>
            </div>
            <div class="actions">
              <t-button variant="outline" size="small" @click="copyEnrollmentToken">复制令牌</t-button>
              <t-button variant="outline" size="small" @click="copyEnrollmentConfig">复制完整配置</t-button>
            </div>
          </div>
          <t-loading :loading="loadingEdge">
            <template v-if="enterpriseEdge">
              <div class="edge-summary">
                <span><small>节点</small><strong>{{ enterpriseEdge.summary.onlineNodeCount }} / {{ enterpriseEdge.summary.nodeCount }} 在线</strong></span>
                <span><small>连接状态</small><strong>{{ edgeStatusLabel(enterpriseEdge.summary.connectionStatus) }}</strong></span>
                <span><small>最近心跳</small><strong>{{ formatTimestamp(enterpriseEdge.summary.lastSeenAt) }}</strong></span>
              </div>
              <div v-if="enterpriseEdge.nodes.length" class="table-scroll">
                <table>
                  <thead><tr><th>节点</th><th>状态</th><th>版本</th><th>Catalog</th><th>数据服务</th><th>最近心跳</th></tr></thead>
                  <tbody>
                    <tr v-for="node in enterpriseEdge.nodes" :key="node.edgeNodeId">
                      <td><strong>{{ node.displayName || node.edgeNodeId }}</strong><small>{{ node.edgeNodeId }}</small></td>
                      <td><t-tag :theme="edgeStatusTheme(node.status)">{{ edgeStatusLabel(node.status) }}</t-tag></td>
                      <td>{{ node.version || '—' }}</td>
                      <td>{{ node.catalogVersion || '未发布' }}</td>
                      <td>{{ dataServiceStatusLabel(node.dataServiceStatus) }}</td>
                      <td>{{ formatTimestamp(node.lastSeenAt) }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
              <t-empty v-else description="尚未接入边缘节点，可生成接入令牌开始部署。" />
            </template>
          </t-loading>
        </t-card>

        <t-card title="企业成员">
          <template #actions><t-button variant="outline" size="small" :disabled="enterpriseMemberActionsDisabled" @click="employeeOpen = true">新增员工</t-button></template>
          <t-alert
            v-if="memberActionsUnavailableReason"
            theme="warning"
            :message="memberActionsUnavailableReason"
          />
          <div class="member-toolbar">
            <t-input v-model="memberQueryDraft" clearable placeholder="搜索用户名或邮箱" @enter="searchMembers" />
            <t-button variant="outline" @click="searchMembers">搜索</t-button>
          </div>
          <t-loading :loading="loadingMembers">
            <div class="table-scroll">
              <table>
                <thead><tr><th>成员</th><th>角色</th><th>状态</th><th>操作</th></tr></thead>
                <tbody>
                  <tr v-for="member in members" :key="member.user_id">
                    <td><strong>{{ member.username }}</strong><small>{{ member.email }}</small></td>
                    <td>
                      <t-select :value="member.role" size="small" :disabled="enterpriseMemberActionsDisabled" @change="(value: unknown) => changeRole(member, String(value) as 'admin' | 'viewer')">
                        <t-option value="admin" label="管理员" /><t-option value="viewer" label="员工" />
                      </t-select>
                    </td>
                    <td><t-tag :theme="member.status === 'active' ? 'success' : 'default'">{{ member.status === 'active' ? '正常' : '停用' }}</t-tag></td>
                    <td class="actions">
                      <t-button variant="text" size="small" :disabled="enterpriseMemberActionsDisabled" @click="toggleStatus(member)">{{ member.status === 'active' ? '停用' : '恢复' }}</t-button>
                      <t-button variant="text" size="small" :disabled="enterpriseMemberActionsDisabled" @click="openPasswordReset(member)">重置密码</t-button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-if="memberTotal > memberPageSize" class="member-pagination">
              <span>第 {{ memberPage }} / {{ memberPageCount }} 页，共 {{ memberTotal }} 人</span>
              <div class="actions">
                <t-button size="small" variant="outline" :disabled="memberPage <= 1" @click="changeMemberPage(memberPage - 1)">上一页</t-button>
                <t-button size="small" variant="outline" :disabled="memberPage >= memberPageCount" @click="changeMemberPage(memberPage + 1)">下一页</t-button>
              </div>
            </div>
          </t-loading>
        </t-card>

      </div>
      <t-empty v-else class="detail-empty" description="选择一个企业查看详情" />
    </section>

    <t-dialog v-model:visible="creationOpen" header="创建企业" width="640px" :footer="false">
      <ol class="wizard-steps" aria-label="创建企业步骤">
        <li v-for="(label, index) in creationStepLabels" :key="label" :class="{ active: creationStep >= index, current: creationStep === index }">
          <span>{{ index + 1 }}</span>{{ label }}
        </li>
      </ol>

      <t-alert
        v-if="creationLocked"
        theme="warning"
        :message="activationLocked
          ? `初始管理员已创建（${initialAdminUserID}），开通参数已锁定。可用同一开通编号查询并继续。`
          : `初始管理员已创建（${initialAdminUserID}），管理员资料已锁定；可返回补齐企业资料后继续开通。`"
      />
      <t-alert
        v-if="activationStatusMessage"
        :theme="activationStatus === 'abandoned' ? 'error' : 'info'"
        :message="activationStatus === 'abandoned'
          ? `${activationStatusMessage}。关闭只会结束本次浏览器流程，服务器上的管理员记录仍会保留。`
          : activationStatusMessage"
      />
      <t-alert
        v-if="initialAdminPasswordUnknown"
        theme="warning"
        message="服务器已确认初始管理员创建成功，但浏览器没有持久化原密码。企业开通后，请立即在成员列表为该管理员显式重置密码。"
      />

      <t-form label-align="top" class="creation-form">
        <template v-if="creationStep === 0">
          <t-form-item label="企业名称"><t-input v-model="creation.name" :disabled="activationLocked" /></t-form-item>
          <t-form-item label="企业说明"><t-textarea v-model="creation.description" :disabled="activationLocked" /></t-form-item>
        </template>
        <template v-else-if="creationStep === 1">
          <div class="form-grid">
            <t-form-item label="席位总数"><t-input-number v-model="creation.seats_total" :min="1" :disabled="activationLocked" /></t-form-item>
            <t-form-item label="存储配额（GiB）"><t-input-number v-model="creation.storage_quota_gib" :min="0" :disabled="activationLocked" /></t-form-item>
          </div>
          <p class="hint">配额覆盖企业知识文件、索引和相关数据；0 表示不限额。</p>
        </template>
        <template v-else>
          <div class="form-grid">
            <t-form-item label="用户名"><t-input v-model="creation.username" :disabled="creationLocked" /></t-form-item>
            <t-form-item label="邮箱"><t-input v-model="creation.email" :disabled="creationLocked" /></t-form-item>
          </div>
          <t-form-item label="初始密码">
            <t-input v-model="creation.password" type="password" :disabled="creationLocked" autocomplete="new-password" />
          </t-form-item>
          <p class="hint">密码只用于本次请求，不写入浏览器存储。</p>
        </template>
      </t-form>

      <div class="dialog-actions">
        <t-button variant="outline" :disabled="creationStep === 0 || activationLocked" @click="creationStep--">上一步</t-button>
        <span class="dialog-actions__spacer" />
        <t-button v-if="activationStatus === 'abandoned'" theme="danger" @click="restartAfterAbandonedActivation">关闭此失败命令并开始新的企业创建</t-button>
        <template v-else>
          <t-button v-if="creationLocked && pendingActivationCommand" variant="outline" :loading="recoveringActivation" @click="recoverActivation">查询开通状态</t-button>
          <t-button v-if="creationStep < 2" theme="primary" @click="goToNextCreationStep">下一步</t-button>
          <t-button v-else theme="primary" :loading="creating" @click="createEnterprise">{{ creationLocked ? (initialAdminPasswordUnknown ? '继续开通并随后重置密码' : '继续开通') : '创建并开通' }}</t-button>
        </template>
      </div>
    </t-dialog>

    <t-dialog v-model:visible="employeeOpen" header="新增员工" :confirm-btn="{ content: '新增', loading: creatingEmployee }" @confirm="createEmployee">
      <t-form label-align="top">
        <t-form-item label="用户名"><t-input v-model="employee.username" /></t-form-item>
        <t-form-item label="邮箱"><t-input v-model="employee.email" /></t-form-item>
        <t-form-item label="初始密码"><t-input v-model="employee.password" type="password" /></t-form-item>
      </t-form>
    </t-dialog>

    <t-dialog v-model:visible="passwordOpen" header="重置密码" @confirm="resetPassword">
      <t-input v-model="newPassword" type="password" placeholder="输入新密码" />
    </t-dialog>

    <Settings :tenant-control-id="selected?.id" :tenant-control-name="selected?.name" />
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import Settings from '@/views/settings/Settings.vue'
import { useUIStore } from '@/stores/ui'
import {
  activateEnterprise, createInitialAdministrator, createOperationsEmployee, getEnterpriseActivation,
  getInitialAdministratorCommand,
  getOperationsEnterprise, getOperationsEnterpriseEdge, listOperationsEnterprises, listOperationsMembers,
  resetOperationsMemberPassword, rotateOperationsEnrollmentToken,
  updateOperationsEnterprise, updateOperationsMemberRole, updateOperationsMemberStatus,
  type EnterpriseActivation, type OperationsEnterprise, type OperationsEnterpriseEdge, type OperationsMember,
} from '@/api/platformOperations'
import {
  activationPayload,
  bytesToGiB,
  createActivationCommand,
  enterpriseFormDraft,
  enterpriseUpdatePayload,
  freshEnterpriseCreationDraft,
  initialAdministratorCommand,
  PENDING_ACTIVATION_STORAGE_KEY,
  PENDING_INITIAL_ADMIN_STORAGE_KEY,
  type ActivationCommand,
  type InitialAdministratorCommand,
} from './platformOperationsModel'

const uiStore = useUIStore()
const enterprises = ref<OperationsEnterprise[]>([])
const selected = ref<OperationsEnterprise>()
const members = ref<OperationsMember[]>([])
const enterpriseEdge = ref<OperationsEnterpriseEdge>()
const memberPage = ref(1), memberPageSize = 20, memberTotal = ref(0)
const memberQuery = ref(''), memberQueryDraft = ref('')
const memberPageCount = computed(() => Math.max(1, Math.ceil(memberTotal.value / memberPageSize)))
const loadingEnterprises = ref(false), loadingMembers = ref(false)
const loadingEdge = ref(false), rotatingEnrollmentToken = ref(false)
const savingEnterprise = ref(false), creating = ref(false), creatingEmployee = ref(false)
const creationOpen = ref(false), employeeOpen = ref(false), passwordOpen = ref(false)
const passwordTarget = ref<OperationsMember>(), newPassword = ref(''), errorMessage = ref('')
const passwordResetNotice = ref('')
const edgeErrorMessage = ref(''), enrollmentToken = ref('')
const suggestedEdgeNodeID = computed(() => {
  const source = String(selected.value?.name || selected.value?.id || 'enterprise').trim().toLowerCase()
  const slug = source.replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
  return `${slug || `tenant-${selected.value?.id || 'enterprise'}`}-edge-001`
})
const edgeEnrollmentConfig = computed(() => [
  `CENTER_URL=${window.location.origin}`,
  `TENANT_ID=${enterpriseEdge.value?.enterpriseId || ''}`,
  `EDGE_NODE_ID=${suggestedEdgeNodeID.value}`,
  `EDGE_DISPLAY_NAME=${selected.value?.name || suggestedEdgeNodeID.value}`,
  'EDGE_BASE_DATA_DIR=/data/edge-agent',
  `EDGE_ENROLLMENT_TOKEN=${enrollmentToken.value}`,
].join('\n'))
const initialAdminUserID = ref('')
const pendingInitialAdminCommand = ref<InitialAdministratorCommand | null>(null)
const initialAdminPasswordUnknown = ref(false)
const creationStep = ref(0)
const creationStepLabels = ['企业资料', '配额', '初始管理员']
const pendingActivationCommand = ref<ActivationCommand | null>(null)
const activationStatus = ref<EnterpriseActivation['status']>()
const activationStatusMessage = ref('')
const recoveringActivation = ref(false)
const creationLocked = computed(() => initialAdminUserID.value !== '')
const activationLocked = computed(() => pendingActivationCommand.value !== null)
const enterpriseMemberActionsDisabled = computed(() => selected.value?.status !== 'active')
const memberActionsUnavailableReason = computed(() => {
  if (selected.value?.status === 'suspended') return '企业已暂停，成员新增、角色、状态和密码操作不可用。请先重新启用企业。'
  if (selected.value?.status === 'provisioning') return '企业仍在开通中，成员角色、状态和密码操作暂不可用。'
  if (selected.value?.status === 'activation_abandoned') return '企业开通已放弃，成员角色、状态和密码操作不可用。'
  return ''
})
const editEnterprise = reactive<{ name: string; description: string; status: OperationsEnterprise['status']; seats_total: number | null; storage_quota_gib: number }>({ name: '', description: '', status: 'active', seats_total: null, storage_quota_gib: 10 })
const creation = reactive(freshEnterpriseCreationDraft())
const employee = reactive({ username: '', email: '', password: '' })

function showError(error: any) { errorMessage.value = error?.message || '操作失败' }
function openTenantSettings(section: 'sandbox' | 'skills' | 'chathistory' | 'memory-runtime') {
  if (!selected.value) return
  uiStore.openSettings(section)
}
function formatStorage(bytes: number) { return `${bytesToGiB(bytes).toFixed(2)} GiB` }
function formatTimestamp(value: string | null) { return value ? new Date(value).toLocaleString() : '暂无' }
function edgeStatusLabel(status: string) {
  return { online: '在线', offline: '离线', disabled: '已停用', not_connected: '未接入' }[status] || '未知'
}
function edgeStatusTheme(status: string): 'success' | 'warning' | 'default' {
  if (status === 'online') return 'success'
  if (status === 'offline') return 'warning'
  return 'default'
}
function dataServiceStatusLabel(status: Record<string, unknown>) {
  const value = String(status?.status || '')
  return { available: '可用', degraded: '异常', unavailable: '不可用' }[value] || '未知'
}
function edgeErrorText(error: any) {
  if (error?.message === 'enterprise_binding_not_found') return '企业尚未完成边缘服务绑定，完成开通后即可接入节点。'
  if (error?.status === 503) return '节点服务暂不可用，请稍后刷新。'
  return error?.message || '节点信息暂不可用'
}
async function copyEnrollmentToken() {
  if (!enrollmentToken.value) return
  await navigator.clipboard.writeText(enrollmentToken.value)
  MessagePlugin.success('接入令牌已复制')
}
async function copyEnrollmentConfig() {
  if (!enrollmentToken.value) return
  await navigator.clipboard.writeText(edgeEnrollmentConfig.value)
  MessagePlugin.success('节点接入配置已复制')
}
function enterpriseStatusLabel(status: OperationsEnterprise['status']) {
  return { active: '启用', suspended: '暂停', provisioning: '待开通', activation_abandoned: '已放弃' }[status]
}
function enterpriseStatusTheme(status: OperationsEnterprise['status']): 'success' | 'warning' | 'default' {
  if (status === 'active') return 'success'
  if (status === 'suspended' || status === 'provisioning') return 'warning'
  return 'default'
}

function applyEnterpriseDraft(enterprise: OperationsEnterprise) {
  Object.assign(editEnterprise, enterpriseFormDraft(enterprise))
}

async function loadEnterprises(selectID?: number) {
  loadingEnterprises.value = true
  try {
    const response = await listOperationsEnterprises()
    enterprises.value = response.items || []
    const id = selectID ?? selected.value?.id
    if (id) {
      const refreshed = enterprises.value.find(item => item.id === id)
      if (refreshed) await selectEnterprise(refreshed)
    }
  } catch (error) { showError(error) } finally { loadingEnterprises.value = false }
}

async function selectEnterprise(enterprise: OperationsEnterprise) {
  selected.value = enterprise
  applyEnterpriseDraft(enterprise)
  const selectedID = enterprise.id
  memberPage.value = 1
  memberQuery.value = ''
  memberQueryDraft.value = ''
  enterpriseEdge.value = undefined
  edgeErrorMessage.value = ''
  enrollmentToken.value = ''
  const detailPromise = getOperationsEnterprise(selectedID).then((detail) => {
    if (selected.value?.id !== selectedID) return
    selected.value = detail
    applyEnterpriseDraft(detail)
  })
  await Promise.all([detailPromise, loadMembers(), loadEnterpriseEdge()]).catch(showError)
}

async function loadEnterpriseEdge() {
  if (!selected.value) return
  const tenantID = selected.value.id
  loadingEdge.value = true
  edgeErrorMessage.value = ''
  try {
    const result = await getOperationsEnterpriseEdge(tenantID)
    if (selected.value?.id === tenantID) enterpriseEdge.value = result
  } catch (error: any) {
    if (selected.value?.id === tenantID) {
      enterpriseEdge.value = undefined
      edgeErrorMessage.value = edgeErrorText(error)
    }
  } finally {
    if (selected.value?.id === tenantID) loadingEdge.value = false
  }
}

async function rotateEnrollmentToken() {
  if (!selected.value) return
  const tenantID = selected.value.id
  rotatingEnrollmentToken.value = true
  enrollmentToken.value = ''
  try {
    const result = await rotateOperationsEnrollmentToken(tenantID)
    if (selected.value?.id !== tenantID) return
    enrollmentToken.value = result.enrollmentToken
    edgeErrorMessage.value = ''
    await loadEnterpriseEdge()
  } catch (error: any) {
    if (selected.value?.id === tenantID) edgeErrorMessage.value = edgeErrorText(error)
  } finally {
    if (selected.value?.id === tenantID) rotatingEnrollmentToken.value = false
  }
}

async function saveEnterprise() {
  if (!selected.value) return
  if (!editEnterprise.name.trim()) {
    errorMessage.value = '请输入企业名称'
    return
  }
  const payload = enterpriseUpdatePayload(editEnterprise)
  if ((payload.seats_total !== null && payload.seats_total < selected.value.seats_used) || editEnterprise.storage_quota_gib < 0) {
    errorMessage.value = '席位总数不能低于已用席位，存储配额不能小于 0'
    return
  }
  savingEnterprise.value = true
  try {
    const updated = await updateOperationsEnterprise(selected.value.id, payload)
    selected.value = updated
    await loadEnterprises(updated.id)
    MessagePlugin.success('企业信息已保存')
  } catch (error) { showError(error) } finally { savingEnterprise.value = false }
}

async function loadMembers() {
  if (!selected.value) return
  const tenantID = selected.value.id
  loadingMembers.value = true
  try {
    const response = await listOperationsMembers(tenantID, memberPage.value, memberPageSize, memberQuery.value)
    if (selected.value?.id !== tenantID) return
    members.value = response.items || []
    memberTotal.value = response.total
    memberPage.value = response.page
  }
  catch (error) { showError(error) } finally { loadingMembers.value = false }
}

function searchMembers() {
  memberQuery.value = memberQueryDraft.value.trim()
  memberPage.value = 1
  void loadMembers()
}

function changeMemberPage(page: number) {
  if (page < 1 || page > memberPageCount.value || page === memberPage.value) return
  memberPage.value = page
  void loadMembers()
}

async function createEmployee() {
  if (!selected.value || enterpriseMemberActionsDisabled.value) return
  creatingEmployee.value = true
  try {
    await createOperationsEmployee(selected.value.id, employee)
    employeeOpen.value = false
    Object.assign(employee, { username: '', email: '', password: '' })
    memberPage.value = 1
    await loadEnterprises(selected.value.id)
  } catch (error) { showError(error) } finally { creatingEmployee.value = false }
}

async function changeRole(member: OperationsMember, role: OperationsMember['role']) {
  if (!selected.value || enterpriseMemberActionsDisabled.value || role === member.role) return
  try { await updateOperationsMemberRole(selected.value.id, member.user_id, role); await loadMembers() }
  catch (error) { showError(error) }
}

async function toggleStatus(member: OperationsMember) {
  if (!selected.value || enterpriseMemberActionsDisabled.value) return
  const status = member.status === 'active' ? 'suspended' : 'active'
  try { await updateOperationsMemberStatus(selected.value.id, member.user_id, status); await loadEnterprises(selected.value.id) }
  catch (error) { showError(error) }
}

function openPasswordReset(member: OperationsMember) {
  if (enterpriseMemberActionsDisabled.value) return
  passwordTarget.value = member
  newPassword.value = ''
  passwordOpen.value = true
}
async function resetPassword() {
  if (!selected.value || !passwordTarget.value || enterpriseMemberActionsDisabled.value) return
  try { await resetOperationsMemberPassword(selected.value.id, passwordTarget.value.user_id, newPassword.value); passwordOpen.value = false; MessagePlugin.success('密码已重置') }
  catch (error) { showError(error) }
}

async function createEnterprise() {
  if (!validateCreationStep(2)) return
  creating.value = true
  try {
    if (!initialAdminUserID.value) {
      pendingInitialAdminCommand.value = initialAdministratorCommand(
        pendingInitialAdminCommand.value,
        creation.username,
        creation.email,
        () => crypto.randomUUID(),
      )
      persistPendingInitialAdministrator()
      const command = pendingInitialAdminCommand.value
      let admin
      try {
        admin = await createInitialAdministrator(command.commandId, {
          username: command.username,
          email: command.email,
          password: creation.password,
        })
      } catch (createError) {
        try {
          admin = await getInitialAdministratorCommand(command.commandId)
          initialAdminPasswordUnknown.value = true
        } catch {
          throw createError
        }
      }
      initialAdminUserID.value = admin.user_id
      if (admin.replayed && !creation.password) initialAdminPasswordUnknown.value = true
      creation.password = ''
    }
    if (!pendingActivationCommand.value) {
      pendingActivationCommand.value = createActivationCommand(
        activationPayload(creation, initialAdminUserID.value),
        () => crypto.randomUUID(),
      )
      persistPendingActivation()
    }
    const command = pendingActivationCommand.value
    const activation = await activateEnterprise(command.activationId, command.idempotencyKey, command.payload)
    await applyActivation(activation)
  } catch (error) {
    if (pendingActivationCommand.value) {
      try {
        const activation = await getEnterpriseActivation(pendingActivationCommand.value.activationId)
        await applyActivation(activation)
        if (activation.status !== 'completed') showError(error)
      } catch {
        showError(error)
      }
    } else {
      showError(error)
    }
  } finally { creating.value = false }
}

function validateCreationStep(step: number) {
  if (step >= 0 && !creation.name.trim()) {
    errorMessage.value = '请输入企业名称'
    creationStep.value = 0
    return false
  }
  if (step >= 1 && (!Number.isFinite(creation.seats_total) || creation.seats_total < 1 || !Number.isFinite(creation.storage_quota_gib) || creation.storage_quota_gib < 0)) {
    errorMessage.value = '席位总数必须大于 0，存储配额不能小于 0'
    creationStep.value = 1
    return false
  }
  if (step >= 2 && !creationLocked.value && (!creation.username.trim() || !creation.email.trim() || !creation.email.includes('@') || !creation.password)) {
    errorMessage.value = '请输入有效的初始管理员用户名、邮箱和密码'
    creationStep.value = 2
    return false
  }
  errorMessage.value = ''
  return true
}

function goToNextCreationStep() {
  if (validateCreationStep(creationStep.value)) creationStep.value++
}

function openCreationWizard() {
  if (!pendingActivationCommand.value) creationStep.value = 0
  creationOpen.value = true
}

function persistPendingActivation() {
  if (!pendingActivationCommand.value) return
  sessionStorage.setItem(PENDING_ACTIVATION_STORAGE_KEY, JSON.stringify(pendingActivationCommand.value))
}

function persistPendingInitialAdministrator() {
  if (!pendingInitialAdminCommand.value) return
  sessionStorage.setItem(PENDING_INITIAL_ADMIN_STORAGE_KEY, JSON.stringify(pendingInitialAdminCommand.value))
}

function readPendingInitialAdministrator(): InitialAdministratorCommand | null {
  const raw = sessionStorage.getItem(PENDING_INITIAL_ADMIN_STORAGE_KEY)
  if (!raw) return null
  try {
    const value = JSON.parse(raw) as InitialAdministratorCommand
    if (
      !value || typeof value.commandId !== 'string' || typeof value.username !== 'string'
      || typeof value.email !== 'string'
    ) throw new Error('invalid initial administrator command')
    return value
  } catch {
    sessionStorage.removeItem(PENDING_INITIAL_ADMIN_STORAGE_KEY)
    return null
  }
}

function readPendingActivation(): ActivationCommand | null {
  const raw = sessionStorage.getItem(PENDING_ACTIVATION_STORAGE_KEY)
  if (!raw) return null
  try {
    const value = JSON.parse(raw) as ActivationCommand
    if (
      !value || typeof value.activationId !== 'string' || typeof value.idempotencyKey !== 'string'
      || !value.payload || typeof value.payload.initial_administrator_user_id !== 'string'
    ) {
      sessionStorage.removeItem(PENDING_ACTIVATION_STORAGE_KEY)
      return null
    }
    return value
  } catch {
    sessionStorage.removeItem(PENDING_ACTIVATION_STORAGE_KEY)
    return null
  }
}

function activationMessage(activation: EnterpriseActivation) {
  const labels: Record<EnterpriseActivation['status'], string> = {
    pending: '等待开通',
    pb_prepared: '企业空间已准备',
    ready: '等待完成',
    completed: '开通完成',
    abandoned: '开通已终止',
  }
  const error = activation.lastErrorCode ? `，错误码：${activation.lastErrorCode}` : ''
  return `开通编号 ${activation.activationId}：${labels[activation.status]}${error}`
}

async function applyActivation(activation: EnterpriseActivation) {
  activationStatus.value = activation.status
  activationStatusMessage.value = activationMessage(activation)
  if (activation.status !== 'completed') return
  const tenantID = activation.productBaseTenantId === null ? undefined : Number(activation.productBaseTenantId)
  const recoveredAdministratorLabel = creation.username || creation.email || activation.initialAdministratorUserId
  sessionStorage.removeItem(PENDING_ACTIVATION_STORAGE_KEY)
  sessionStorage.removeItem(PENDING_INITIAL_ADMIN_STORAGE_KEY)
  pendingActivationCommand.value = null
  pendingInitialAdminCommand.value = null
  initialAdminUserID.value = ''
  activationStatus.value = undefined
  activationStatusMessage.value = ''
  creationOpen.value = false
  creationStep.value = 0
  Object.assign(creation, freshEnterpriseCreationDraft())
  await loadEnterprises(Number.isSafeInteger(tenantID) ? tenantID : undefined)
  if (initialAdminPasswordUnknown.value) {
    passwordResetNotice.value = `企业已开通。初始管理员 ${recoveredAdministratorLabel} 的原密码未保留，请在成员列表中立即执行“重置密码”。`
  } else {
    MessagePlugin.success('企业已开通')
  }
  initialAdminPasswordUnknown.value = false
}

function restartAfterAbandonedActivation() {
  if (activationStatus.value !== 'abandoned') return
  sessionStorage.removeItem(PENDING_ACTIVATION_STORAGE_KEY)
  sessionStorage.removeItem(PENDING_INITIAL_ADMIN_STORAGE_KEY)
  pendingActivationCommand.value = null
  pendingInitialAdminCommand.value = null
  initialAdminUserID.value = ''
  initialAdminPasswordUnknown.value = false
  activationStatus.value = undefined
  activationStatusMessage.value = ''
  errorMessage.value = ''
  Object.assign(creation, freshEnterpriseCreationDraft())
  creationStep.value = 0
  creationOpen.value = true
}

async function recoverActivation() {
  if (!pendingActivationCommand.value) return
  recoveringActivation.value = true
  try {
    await applyActivation(await getEnterpriseActivation(pendingActivationCommand.value.activationId))
  } catch (error) {
    showError(error)
  } finally {
    recoveringActivation.value = false
  }
}

async function restorePendingActivation() {
  const command = readPendingActivation()
  if (!command) return
  pendingActivationCommand.value = command
  initialAdminUserID.value = command.payload.initial_administrator_user_id
  initialAdminPasswordUnknown.value = true
  Object.assign(creation, {
    name: command.payload.name,
    description: command.payload.description,
    seats_total: command.payload.seats_total,
    storage_quota_gib: bytesToGiB(command.payload.storage_quota),
    username: '',
    email: '',
    password: '',
  })
  creationStep.value = 2
  creationOpen.value = true
  await recoverActivation()
}

async function restorePendingInitialAdministrator() {
  const command = readPendingInitialAdministrator()
  if (!command || pendingActivationCommand.value) return
  pendingInitialAdminCommand.value = command
  Object.assign(creation, { username: command.username, email: command.email, password: '' })
  creationStep.value = 2
  creationOpen.value = true
  try {
    const admin = await getInitialAdministratorCommand(command.commandId)
    initialAdminUserID.value = admin.user_id
    initialAdminPasswordUnknown.value = true
  } catch (error) {
    showError(error)
  }
}

onMounted(async () => {
  await loadEnterprises()
  await restorePendingActivation()
  await restorePendingInitialAdministrator()
})
</script>

<style scoped lang="less">
.operations-page { width: min(1240px, calc(100% - 48px)); margin: 0 auto; padding: 38px 0 72px; color: var(--td-text-color-primary); }
.page-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 24px; margin-bottom: 24px; }
.page-header h1 { margin: 4px 0 8px; font-size: 32px; }.page-header p { margin: 0; color: var(--td-text-color-secondary); }.eyebrow { color: var(--td-brand-color) !important; font-weight: 700; }
.page-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.tenant-settings-hint { flex-basis: 100%; color: var(--td-text-color-placeholder); text-align: right; }
.workspace { display: grid; grid-template-columns: 280px minmax(0, 1fr); gap: 18px; margin-top: 18px; }.detail-column { display: grid; gap: 18px; }.detail-empty { padding-top: 100px; }
.enterprise-row { width: 100%; display: flex; justify-content: space-between; align-items: center; gap: 12px; padding: 13px; border: 0; border-radius: 8px; background: transparent; text-align: left; color: inherit; cursor: pointer; }.enterprise-row:hover, .enterprise-row.active { background: var(--td-bg-color-container-hover); }.enterprise-row span:first-child, td:first-child { display: grid; gap: 4px; }.enterprise-row small, td small, .hint { color: var(--td-text-color-secondary); }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 14px; }.table-scroll { overflow-x: auto; }table { width: 100%; border-collapse: collapse; }th, td { padding: 12px; border-bottom: 1px solid var(--td-component-stroke); text-align: left; }.actions { display: flex; flex-wrap: wrap; gap: 8px; }.secret { margin: 16px 0; padding: 14px; border-radius: 8px; background: var(--td-bg-color-secondarycontainer); white-space: pre-wrap; overflow-wrap: anywhere; }
.member-toolbar { display: grid; grid-template-columns: minmax(0, 320px) auto; gap: 8px; margin-bottom: 12px; }.member-pagination { display: flex; justify-content: space-between; align-items: center; gap: 12px; padding-top: 14px; color: var(--td-text-color-secondary); }
.enterprise-row span.enterprise-row__summary { display: flex; align-items: flex-end; gap: 5px; }
.edge-summary { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; margin: 4px 0 18px; }
.edge-summary span { display: grid; gap: 5px; padding: 12px; border-radius: 8px; background: var(--td-bg-color-secondarycontainer); }
.edge-summary small { color: var(--td-text-color-secondary); }
.secret-row { display: grid; gap: 10px; margin: 16px 0; }.secret-row .secret { margin: 5px 0; }
.wizard-steps { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; margin: 0 0 22px; padding: 0; list-style: none; color: var(--td-text-color-placeholder); }
.wizard-steps li { display: flex; align-items: center; gap: 7px; padding-bottom: 9px; border-bottom: 2px solid var(--td-component-stroke); }
.wizard-steps li span { display: grid; width: 22px; height: 22px; place-items: center; border-radius: 50%; background: var(--td-bg-color-secondarycontainer); font-size: 12px; }
.wizard-steps li.active { color: var(--td-text-color-primary); border-color: var(--td-brand-color-4); }.wizard-steps li.current { color: var(--td-brand-color); font-weight: 600; border-color: var(--td-brand-color); }
.creation-form { min-height: 190px; margin-top: 18px; }.dialog-actions { display: flex; align-items: center; gap: 8px; margin-top: 22px; }.dialog-actions__spacer { flex: 1; }
@media (max-width: 820px) { .operations-page { width: calc(100% - 28px); }.workspace { grid-template-columns: 1fr; }.form-grid, .edge-summary { grid-template-columns: 1fr; } }
</style>
