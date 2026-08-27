<template>
  <t-dialog :visible="visible" :header="t('knowledgeAccess.title')" :confirm-btn="{ content: t('common.save'), loading, disabled: !loaded || loading }" @confirm="save" @close="close">
    <p class="scope-hint">{{ t('knowledgeAccess.description') }}</p>
    <t-radio-group v-model="mode">
      <t-radio value="all">{{ t('knowledgeAccess.all') }}</t-radio>
      <t-radio value="roles">{{ t('knowledgeAccess.roles') }}</t-radio>
    </t-radio-group>
    <t-checkbox-group v-if="mode === 'roles'" v-model="roleIDs" class="scope-roles">
      <t-checkbox v-for="role in roles" :key="role.id" :value="role.id" :disabled="!role.enabled && !roleIDs.includes(role.id)">{{ role.name }}<span v-if="!role.enabled">（{{ t('knowledgeAccess.disabled') }}）</span></t-checkbox>
    </t-checkbox-group>
    <t-empty v-if="mode === 'roles' && !roles.some((item) => item.enabled)" :description="t('knowledgeAccess.noRoles')" />
  </t-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import { getKnowledgeAccessScope, listBusinessRoles, updateKnowledgeAccessScope, type BusinessRole } from '@/api/business-role'

const props = defineProps<{ visible: boolean; kbId: string }>()
const { t } = useI18n()
const emit = defineEmits<{ 'update:visible': [value: boolean]; saved: [] }>()
const roles = ref<BusinessRole[]>([])
const roleIDs = ref<string[]>([])
const mode = ref<'all' | 'roles'>('all')
const loading = ref(false)
const loaded = ref(false)
let loadVersion = 0

const close = () => emit('update:visible', false)
const load = async () => {
  const version = ++loadVersion
  if (!props.kbId) return
  loading.value = true
  try {
    const [rolesResponse, accessResponse] = await Promise.all([listBusinessRoles(), getKnowledgeAccessScope(props.kbId)])
    if (version !== loadVersion || !props.visible) return
    roles.value = rolesResponse?.data || []
    mode.value = accessResponse?.data?.mode || 'all'
    roleIDs.value = accessResponse?.data?.role_ids || []
    loaded.value = true
  } catch {
    if (version === loadVersion && props.visible) MessagePlugin.error(t('knowledgeAccess.loadFailed'))
  } finally {
    if (version === loadVersion) loading.value = false
  }
}
const save = async () => {
  if (!loaded.value || loading.value) return
  if (mode.value === 'roles' && roleIDs.value.length === 0) {
    MessagePlugin.warning(t('knowledgeAccess.roleRequired'))
    return
  }
  loading.value = true
  try {
    await updateKnowledgeAccessScope(props.kbId, { mode: mode.value, role_ids: mode.value === 'all' ? [] : roleIDs.value })
    MessagePlugin.success(t('knowledgeAccess.saved'))
    emit('saved')
    close()
  } catch {
    MessagePlugin.error(t('knowledgeAccess.saveFailed'))
  } finally {
    loading.value = false
  }
}
watch(() => [props.visible, props.kbId], ([open]) => {
  if (!open) {
    loadVersion++
    loaded.value = false
    loading.value = false
    return
  }
  roles.value = []
  roleIDs.value = []
  mode.value = 'all'
  loaded.value = false
  void load()
}, { immediate: true })
</script>

<style scoped>
.scope-hint { color: var(--td-text-color-secondary); line-height: 1.65; margin: 0 0 16px; }
.scope-roles { display: grid; gap: 12px; margin-top: 16px; }
</style>
