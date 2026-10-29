<template>
  <n-modal :show="show" preset="card" :title="text.bulkTitle" :style="{ width: 'min(680px, 96vw)' }" :mask-closable="!running" :close-on-esc="!running" :closable="!running" @update:show="close">
    <p>{{ tableName }} · {{ capturedKeys.length }}</p>
    <div v-for="col in editableColumns" :key="col.name" class="bulk-edit__field">
      <n-checkbox :checked="selected[col.name] === true" :disabled="locked" @update:checked="selected[col.name] = $event">{{ col.name }}</n-checkbox>
      <n-select v-if="col.enumValues?.length || isRowBoolean(col.type)" :value="String(fields[col.name]?.value ?? '')" :disabled="!selected[col.name] || fields[col.name]?.mode === 'null' || locked" :options="col.enumValues?.map(value => ({label:value,value})) ?? [{label:'true',value:'true'},{label:'false',value:'false'}]" @update:value="setValue(col, $event)" />
      <n-input v-else :value="String(fields[col.name]?.value ?? '')" :type="/text|json|blob|bytea/i.test(col.type) ? 'textarea' : 'text'" :disabled="!selected[col.name] || fields[col.name]?.mode === 'null' || locked" @update:value="setValue(col, $event)" />
      <n-checkbox v-if="col.nullable" :checked="fields[col.name]?.mode === 'null'" :disabled="!selected[col.name] || locked" @update:checked="fields[col.name]!.mode = $event ? 'null' : 'value'">NULL</n-checkbox>
    </div>
    <p v-if="reviewing" class="bulk-edit__confirmation">{{ text.confirmText.replace('{count}', String(capturedKeys.length)).replace('{table}', tableName) }}</p>
    <p v-if="started" class="bulk-edit__progress" role="status">{{ text.progress.replace('{applied}', String(applied)).replace('{total}', String(capturedKeys.length)) }}</p>
    <p v-if="stopped">{{ text.stopped }}</p>
    <p v-if="error" role="alert" class="bulk-edit__error">{{ error }}</p>
    <template #footer><div class="bulk-edit__footer">
      <n-button v-if="running" @click="stopRequested = true" :disabled="stopRequested">{{ text.stop }}</n-button>
      <n-button v-else @click="close">{{ text.close }}</n-button>
      <n-button v-if="reviewing && !started" data-testid="bulk-confirm" type="warning" @click="apply">{{ text.confirm }}</n-button>
      <n-button v-else-if="!started" data-testid="bulk-review" type="primary" :disabled="!canReview" @click="review">{{ text.review }}</n-button>
    </div></template>
  </n-modal>
</template>
<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NCheckbox, NInput, NModal, NSelect } from 'naive-ui'
import { api } from '../api/client'
import type { ColumnDef } from '../api/types'
import { isRowBoolean, rowField, rowJSONError, rowPayload, rowValueError, type RowField } from '../lib/rowValues'
import { rowWorkflowMessages } from '../i18n/rowWorkflows'
const props = defineProps<{show: boolean; rowsPath: string; columns: ColumnDef[]; rowKeys: Record<string, unknown>[]}>()
const emit = defineEmits<{'update:show': [show: boolean]; saved: []}>()
const { locale } = useI18n()
const text = computed(() => rowWorkflowMessages[locale.value === 'ru' ? 'ru' : 'en'].rowWorkflows)
const tableName = computed(() => { const path = capturedPath.value.split('?')[0]!; const match = path.match(/\/tables\/([^/]+)/); return match ? decodeURIComponent(match[1]!) : path })
const capturedKeys = ref<Record<string, unknown>[]>([])
const capturedPath = ref('')
const selected = reactive<Record<string, boolean>>({})
const fields = reactive<Record<string, RowField>>({})
const editableColumns = computed(() => props.columns.filter(col => !col.generated && !col.autoIncrement))
const reviewing = ref(false)
const running = ref(false)
const started = ref(false)
const stopRequested = ref(false)
const stopped = ref(false)
const applied = ref(0)
const error = ref('')
let pending: Record<string, unknown> = {}
const locked = computed(() => reviewing.value || started.value)
const selectedColumns = computed(() => editableColumns.value.filter(col => selected[col.name]))
const canReview = computed(() => selectedColumns.value.length > 0 && capturedKeys.value.length > 0)
watch(() => props.show, show => {
 if (!show) { if (running.value) stopRequested.value = true; return }
 capturedKeys.value = JSON.parse(JSON.stringify(props.rowKeys))
 capturedPath.value = props.rowsPath
 for (const name of Object.keys(selected)) delete selected[name]
 for (const name of Object.keys(fields)) delete fields[name]
 for (const col of props.columns) fields[col.name] = { ...rowField(col, null, true), mode: 'value' }
 reviewing.value = false; started.value = false; stopped.value = false; stopRequested.value = false; applied.value = 0; error.value = ''
}, { immediate: true })
function setValue(col: ColumnDef, value: string): void { fields[col.name] = {value: isRowBoolean(col.type) ? value === 'true' : value, mode: 'value'} }
function review(): void {
 if (!canReview.value) return
 const invalidValue = rowValueError(selectedColumns.value, fields)
 if (invalidValue) { error.value = text.value.valueError.replace('{column}', invalidValue); return }
 const invalid = rowJSONError(selectedColumns.value, fields)
 if (invalid) { error.value = text.value.jsonError.replace('{column}', invalid); return }
 pending = rowPayload(selectedColumns.value, fields)
 reviewing.value = true; error.value = ''
}
function close(): void { if (!running.value) emit('update:show', false) }
async function apply(): Promise<void> {
 if (running.value || started.value || !reviewing.value) return
 running.value = true; started.value = true
 try {
  for (const where of capturedKeys.value) {
   if (stopRequested.value) { stopped.value = true; break }
   await api('PATCH', capturedPath.value, { where, values: pending })
   applied.value++
  }
 } catch (err) { error.value = err instanceof Error ? err.message : String(err) }
 finally { running.value = false; if (applied.value > 0) emit('saved') }
}
</script>
<style scoped>
.bulk-edit__field { display: grid; grid-template-columns: 150px minmax(0, 1fr) auto; gap: 10px; align-items: start; margin-bottom: 12px; }
.bulk-edit__error { color: var(--error-color, #c33); overflow-wrap: anywhere; }
.bulk-edit__footer { display: flex; justify-content: flex-end; gap: 8px; }
@media (max-width: 600px) { .bulk-edit__field { grid-template-columns: 1fr; } }
</style>
