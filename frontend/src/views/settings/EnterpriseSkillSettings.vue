<template>
  <div class="enterprise-skills">
    <h2>{{ t('enterpriseSkills.title') }}</h2>
    <p>{{ t('enterpriseSkills.description') }}</p>
    <t-alert v-if="error" theme="warning">{{ error }}</t-alert>
    <t-loading :loading="loading">
      <t-empty v-if="!loading && !error && skills.length === 0" :description="t('enterpriseSkills.empty')" />
      <div v-for="skill in skills" :key="skill.id" class="skill-row">
        <div><h3>{{ skill.name }}</h3><p>{{ skill.description }}</p><small>{{ t(skill.status === 'ready' ? 'enterpriseSkills.ready' : 'enterpriseSkills.notReady') }}</small></div>
        <t-switch :value="skill.enabled" :disabled="saving === skill.id || skill.status !== 'ready'" :aria-label="t('enterpriseSkills.enable', { name: skill.name })" @change="(value: boolean | string | number) => toggle(skill, Boolean(value))" />
      </div>
    </t-loading>
  </div>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
import { get, patch } from '@/utils/request'
import { useEditorResourcesStore } from '@/stores/editorResources'
type Skill = { id: string; name: string; description: string; status: string; enabled: boolean }
const skills = ref<Skill[]>([])
const loading = ref(false)
const saving = ref('')
const error = ref('')
const resources = useEditorResourcesStore()
async function load() {
  loading.value = true
  try { const res = await get<{ data: Skill[] }>('/api/v1/employee-assistant/skills/manage'); skills.value = res.data; error.value = '' }
  catch { error.value = t('enterpriseSkills.loadError') }
  finally { loading.value = false }
}
async function toggle(skill: Skill, enabled: boolean) {
  saving.value = skill.id
  try { await patch(`/api/v1/employee-assistant/skills/manage/${encodeURIComponent(skill.id)}`, { enabled }); skill.enabled = enabled; resources.invalidate('skills'); error.value = '' }
  catch { error.value = t('enterpriseSkills.saveError') }
  finally { saving.value = '' }
}
onMounted(load)
</script>
<style scoped>
.enterprise-skills { max-width: 900px; padding: 24px; }
.skill-row { display:flex; align-items:center; justify-content:space-between; gap:24px; padding:20px 0; border-bottom:1px solid var(--td-component-border); }
p, small { color:var(--td-text-color-secondary); }
</style>
