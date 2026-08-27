<template>
  <section class="business-roles">
    <div class="heading"><div><h3>{{ t('businessRoles.title') }}</h3><p>{{ t('businessRoles.description') }}</p></div><t-button theme="primary" @click="create">{{ t('businessRoles.create') }}</t-button></div>
    <t-table :data="roles" row-key="id" :columns="columns" :loading="loading">
      <template #enabled="{ row }"><t-switch :model-value="row.enabled" @change="(value: boolean) => save(row, row.name, value)" /></template>
      <template #actions="{ row }"><t-button variant="text" @click="rename(row)">{{ t('businessRoles.rename') }}</t-button></template>
    </t-table>
  </section>
</template>

<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import { createBusinessRole, listBusinessRoles, updateBusinessRole, type BusinessRole } from '@/api/business-role'

const roles = ref<BusinessRole[]>([]); const loading = ref(false)
const { t } = useI18n()
const columns = [{ colKey: 'name', title: t('businessRoles.columns.name') }, { colKey: 'enabled', title: t('businessRoles.columns.enabled'), cell: 'enabled' }, { colKey: 'actions', title: t('businessRoles.columns.actions'), cell: 'actions' }]
const load = async () => { loading.value = true; try { roles.value = (await listBusinessRoles())?.data || [] } finally { loading.value = false } }
const save = async (role: BusinessRole, name: string, enabled: boolean) => { try { await updateBusinessRole(role.id, name, enabled); await load() } catch { MessagePlugin.error(t('businessRoles.saveFailed')) } }
const ask = (title: string, initial = '') => new Promise<string | null>((resolve) => {
  const value = ref(initial); const dialog = DialogPlugin({ header: title, body: () => h('input', { value: value.value, class: 't-input', onInput: (event: Event) => { value.value = (event.target as HTMLInputElement).value } }), onConfirm: () => { const name = value.value.trim(); if (!name) { MessagePlugin.warning(t('businessRoles.nameRequired')); return }; dialog.destroy(); resolve(name) }, onClose: () => { dialog.destroy(); resolve(null) } })
})
const create = async () => { const name = await ask(t('businessRoles.create')); if (!name) return; try { await createBusinessRole(name); await load() } catch { MessagePlugin.error(t('businessRoles.createFailed')) } }
const rename = async (role: BusinessRole) => { const name = await ask(t('businessRoles.rename'), role.name); if (name) await save(role, name, role.enabled) }
onMounted(load)
</script>

<style scoped>
.heading { display: flex; justify-content: space-between; gap: 24px; align-items: flex-start; margin-bottom: 20px; }
h3 { margin: 0 0 6px; } p { margin: 0; color: var(--td-text-color-secondary); }
</style>
