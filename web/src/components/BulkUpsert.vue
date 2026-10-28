<template>
  <section>
    <n-button size="small" @click="show = !show">{{ t('grid.upsert') }}</n-button>
    <form v-if="show" @submit.prevent>
      <p>{{ t('grid.upsertHint') }}</p>
      <n-input v-model:value="source" type="textarea" :autosize="{ minRows: 4 }" placeholder='[{"id": 1, "name": "value"}]' />
      <n-alert v-if="error" type="error">{{ error }}</n-alert>
      <n-popconfirm @positive-click="save">
        <template #trigger><n-button :loading="busy">{{ t('row.save') }}</n-button></template>
        {{ t('grid.upsertConfirm') }}
      </n-popconfirm>
    </form>
  </section>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { NAlert, NButton, NInput, NPopconfirm } from 'naive-ui'
import { api } from '../api/client'
const props = defineProps<{ path: string }>()
const emit = defineEmits<{ saved: [] }>()
const { t } = useI18n()
const show=ref(false),source=ref('[]'),busy=ref(false),error=ref('')
async function save(): Promise<void> {
  if (busy.value) return
  error.value=''
  try {
    const rows: unknown = JSON.parse(source.value)
    if (!Array.isArray(rows) || rows.length===0 || rows.length>1000 || rows.some(row => row===null || typeof row!=='object' || Array.isArray(row) || Object.keys(row).length===0)) throw new Error(t('grid.upsertInvalid'))
    busy.value=true
    await api('POST',props.path,{rows})
    source.value='[]';show.value=false;emit('saved')
  } catch(err) {error.value=err instanceof Error?err.message:String(err)}
  finally {busy.value=false}
}
</script>
<style scoped>form { display:flex; flex-direction:column; gap:8px; margin:12px 0; }</style>
