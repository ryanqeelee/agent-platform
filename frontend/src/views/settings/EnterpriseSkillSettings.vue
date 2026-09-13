<template>
  <div class="enterprise-skills">
    <h2>{{ t('enterpriseSkills.title') }}</h2>
    <p>{{ t('enterpriseSkills.description') }}</p>
    <t-alert v-if="error" theme="warning">{{ error }}</t-alert>
    <t-loading :loading="loading">
      <t-empty v-if="!loading && !error && skills.length === 0" :description="t('enterpriseSkills.empty')" />
      <div v-for="skill in skills" :key="skill.name" class="skill-row">
        <h3>{{ skill.name }}</h3>
        <p>{{ skill.description }}</p>
      </div>
    </t-loading>
  </div>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { listEmployeeSkills, type SkillInfo } from '@/api/skill'

const { t } = useI18n()
const skills = ref<SkillInfo[]>([])
const loading = ref(false)
const error = ref('')
async function load() {
  loading.value = true
  try { const res = await listEmployeeSkills(); skills.value = res.data || []; error.value = '' }
  catch { error.value = t('enterpriseSkills.loadError') }
  finally { loading.value = false }
}
onMounted(load)
</script>
<style scoped>
.enterprise-skills { max-width: 900px; padding: 24px; }
.skill-row { padding:20px 0; border-bottom:1px solid var(--td-component-border); }
p { color:var(--td-text-color-secondary); }
</style>
