<template>
  <ui-modal :show="show" :title="t('grid.upsert')" :closable="!busy"
    :mask-closable="!busy" :close-on-esc="!busy" style="width:min(620px,96vw)" @update:show="close">
    <form class="bulk-upsert" @submit.prevent>
      <p>{{ t('grid.upsertHint') }}</p>
      <ui-input v-model:value="source" type="textarea" :disabled="busy" :autosize="{ minRows: 5, maxRows: 16 }"
        :input-props="{ 'aria-label': 'JSON', spellcheck: false }" placeholder='[{"id": 1, "name": "value"}]' />
      <ui-alert v-if="error" type="error">{{ error }}</ui-alert>
      <footer>
        <ui-button :disabled="busy" @click="close">{{ t('row.cancel') }}</ui-button>
        <ui-popconfirm @positive-click="save">
          <template #trigger><ui-button type="primary" :loading="busy">{{ t('row.save') }}</ui-button></template>
          {{ t('grid.upsertConfirm') }}
        </ui-popconfirm>
      </footer>
    </form>
  </ui-modal>
</template>
<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { UiAlert, UiButton, UiInput, UiModal, UiPopconfirm } from '../ui'
import { api } from '../api/client'
const props = defineProps<{ path: string; show: boolean }>()
const emit = defineEmits<{ saved: []; 'update:show': [value: boolean] }>()
const { t } = useI18n()
const source = ref('[]'), busy = ref(false), error = ref('')
function close(): void {
  if (!busy.value) emit('update:show', false)
}
async function save(): Promise<void> {
  if (busy.value) return
  error.value = ''
  try {
    const rows: unknown = JSON.parse(source.value)
    if (!Array.isArray(rows) || rows.length === 0 || rows.length > 1000 || rows.some(row => row === null || typeof row !== 'object' || Array.isArray(row) || Object.keys(row).length === 0)) {
      throw new Error(t('grid.upsertInvalid'))
    }
    busy.value = true
    await api('POST', props.path, { rows })
    source.value = '[]'
    emit('update:show', false)
    emit('saved')
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    busy.value = false
  }
}
</script>
<style scoped>
.bulk-upsert { display:grid; gap:14px; }
.bulk-upsert p { margin:0; color:var(--db-muted); }
.bulk-upsert :deep(textarea) { font-family:var(--db-mono); }
.bulk-upsert footer { display:flex; gap:8px; justify-content:flex-end; }
</style>
