<template>
  <n-modal :show="show" preset="card" :title="mode === 'insert' ? t('row.insert') : t('row.edit')" class="row-modal" :style="{ width: 'min(720px, 96vw)' }" :mask-closable="!saving" :close-on-esc="!saving" :closable="!saving" @update:show="close">
    <div v-if="loading">{{ t('common.loading') }}</div>
    <div v-else class="row-modal__fields">
      <div v-for="col in columns" :key="col.name" class="row-modal__field">
        <label class="row-modal__label" :for="`row-${col.name}`">{{ col.name }}</label>
        <div class="row-modal__control">
          <span v-if="col.generated">{{ text.generated }}</span>
          <template v-else>
            <n-select v-if="singleFK(col.name)" :id="`row-${col.name}`" :value="selectValue(col.name)" :options="fkOptions(col.name)" remote filterable tag :disabled="disabled(col)" :loading="parentLoading[col.name]" size="small" class="row-modal__input" @search="searchParent(col.name, $event)" @update:value="setValue(col.name, $event)" />
            <n-input-number v-else-if="isRowNumeric(col.type)" :id="`row-${col.name}`" :value="numberValue(col.name)" :disabled="disabled(col)" size="small" class="row-modal__input" @update:value="setValue(col.name, $event)" />
            <n-select v-else-if="col.enumValues?.length || isRowBoolean(col.type)" :id="`row-${col.name}`" :value="isRowBoolean(col.type) ? stringValue(col.name) : selectValue(col.name)" :options="valueOptions(col)" :disabled="disabled(col)" size="small" class="row-modal__input" @update:value="setValue(col.name, $event)" />
            <input v-else-if="dateType(col)" :id="`row-${col.name}`" :type="dateType(col)" :value="dateDisplayValue(col)" :disabled="disabled(col)" step="any" class="row-modal__input row-modal__date" @input="setValue(col.name, ($event.target as HTMLInputElement).value)">
            <n-input v-else :id="`row-${col.name}`" :value="stringValue(col.name)" :type="/text|json|blob|bytea/i.test(col.type) ? 'textarea' : 'text'" :disabled="disabled(col)" size="small" class="row-modal__input" @update:value="setValue(col.name, $event)" />
            <n-button v-if="isRowJSON(col.type)" size="tiny" :disabled="disabled(col)" @click="formatJSON(col)">{{ text.format }}</n-button>
            <template v-if="isRowBinary(col.type)">
              <small>{{ text.binary }}</small>
              <input type="file" :aria-label="col.name" :disabled="disabled(col)" @change="readBinary(col, $event)">
              <small v-if="/^0x(?:[0-9a-f]{2})*$/i.test(stringValue(col.name))">{{ text.binarySize.replace('{count}', String((stringValue(col.name).length - 2) / 2)) }}</small>
            </template>
            <div v-if="singleFK(col.name)" class="row-modal__related">
              <n-select v-if="parentLabelColumns[col.name]" class="row-modal__fk-search-mode" :value="parentSearchModes[col.name] ?? 'label'" :options="[{label: text.searchName.replace('{column}', parentLabelColumns[col.name]!), value: 'label'}, {label: text.searchKey, value: 'key'}]" :disabled="disabled(col)" size="tiny" @update:value="setParentSearchMode(col.name, $event)" />
              <small>{{ parentErrors[col.name] ? text.unavailable : parentLabelColumns[col.name] ? text.manual : text.manualKey }}</small>
              <a :href="relatedHref(singleFK(col.name)!)">{{ text.related }}</a>
            </div>
            <small v-else-if="foreignKeys?.some(fk => fk.columns.length > 1 && fk.columns.includes(col.name))">{{ text.compound }}</small>
          </template>
        </div>
        <div class="row-modal__choices">
          <n-select v-if="mode === 'insert' && !col.generated && (col.default != null || col.autoIncrement)" class="row-modal__mode" :value="fields[col.name]?.mode === 'default' ? 'default' : 'value'" :options="[{ label: text.value, value: 'value' }, { label: text.default, value: 'default' }]" size="small" :disabled="saving" @update:value="setMode(col.name, $event)" />
          <n-checkbox v-if="col.nullable && !col.generated" class="row-modal__null" size="small" :checked="fields[col.name]?.mode === 'null'" :disabled="saving" @update:checked="setMode(col.name, $event ? 'null' : 'value')">{{ t('row.nullToggle') }}</n-checkbox>
          <span v-else-if="!col.generated" class="row-modal__required" :title="text.required">{{ t('row.required') }}</span>
        </div>
      </div>
    </div>
    <p v-if="error" class="row-modal__error" role="alert">{{ error }}</p>
    <template #footer><div class="row-modal__footer">
      <n-button size="small" :disabled="saving" @click="close">{{ t('row.cancel') }}</n-button>
      <n-button size="small" type="primary" :loading="saving" :disabled="!canSave" @click="save">{{ t('row.save') }}</n-button>
    </div></template>
  </n-modal>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NCheckbox, NInput, NInputNumber, NModal, NSelect, useNotification } from 'naive-ui'
import { api } from '../api/client'
import { useCatalogStore } from '../stores/catalog'
import type { ColumnDef, ForeignKey, Page } from '../api/types'
import { isRowNumeric, isRowBinary, isRowJSON, isRowBoolean, rowField, rowPayload, rowJSONError, rowValueError, type RowField, type RowMode, type RowValue } from '../lib/rowValues'
import { rowWorkflowMessages } from '../i18n/rowWorkflows'
const props = defineProps<{ show: boolean; mode: 'insert' | 'edit'; rowsPath: string; columns: ColumnDef[]; row: Record<string, unknown> | null; rowPk: Record<string, unknown> | null; foreignKeys?: ForeignKey[]; loading?: boolean }>()
const emit = defineEmits<{ 'update:show': [show: boolean]; saved: [] }>()
const { t, locale } = useI18n()
const text = computed(() => rowWorkflowMessages[locale.value === 'ru' ? 'ru' : 'en'].rowWorkflows)
const notification = useNotification()
const catalog = useCatalogStore()
const fields = reactive<Record<string, RowField>>({})
let original: Record<string, unknown> = {}
const saving = ref(false)
const error = ref('')
watch(() => [props.show, props.mode, props.row, props.loading] as const, ([show]) => {
 if (!show || props.loading) return
 for (const key of Object.keys(fields)) delete fields[key]
 for (const col of props.columns) fields[col.name] = rowField(col, props.row, props.mode === 'insert')
 original = rowPayload(props.columns, fields)
 error.value = ''
}, { immediate: true })
function stringValue(name: string): string { return String(fields[name]?.value ?? '') }
function selectValue(name: string): string | number | null { const value = fields[name]?.value; return typeof value === 'boolean' ? String(value) : value ?? null }
function numberValue(name: string): number | null { const value = fields[name]?.value; return value === null || value === '' || value === undefined ? null : Number(value) }
function setValue(name: string, value: RowValue): void {
 const col = props.columns.find(col => col.name === name)
 if (col && isRowBoolean(col.type)) value = value === 'true' || value === true
 fields[name] = { value, mode: 'value' }
 error.value = ''
}
function setMode(name: string, mode: RowMode): void { if (fields[name]) fields[name].mode = mode }
function disabled(col: ColumnDef): boolean { return saving.value || fields[col.name]?.mode !== 'value' }
function valueOptions(col: ColumnDef) { return isRowBoolean(col.type) ? [{ label: 'true', value: 'true' }, { label: 'false', value: 'false' }] : (col.enumValues ?? []).map(value => ({ label: value, value })) }
function dateDisplayValue(col: ColumnDef): string {
 const value = stringValue(col.name)
 // PostgreSQL DATE transport is RFC3339. Keep its calendar day for display;
 // the untouched original value remains in fields and the edit snapshot.
 return /^date$/i.test(col.type) ? value.match(/^\d{4}-\d{2}-\d{2}/)?.[0] ?? value : value.replace(' ', 'T')
}
function dateType(col: ColumnDef): 'date' | 'time' | 'datetime-local' | undefined {
 if (/^date$/i.test(col.type)) return 'date'
 if (/^time(?:\(\d+\))?$/i.test(col.type)) return 'time'
 if (/^(datetime|timestamp)(?:\(\d+\))?$/i.test(col.type) && !/[zZ]|[+-]\d{2}:?\d{2}$/.test(stringValue(col.name))) return 'datetime-local'
 return undefined
}
function formatJSON(col: ColumnDef): void {
 try { setValue(col.name, JSON.stringify(JSON.parse(stringValue(col.name)), null, 2)) } catch { error.value = text.value.jsonError.replace('{column}', col.name) }
}
async function readBinary(col: ColumnDef, event: Event): Promise<void> {
 const file = (event.target as HTMLInputElement).files?.[0]
 if (!file) return
 try { const bytes = new Uint8Array(await file.arrayBuffer()); setValue(col.name, '0x' + Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('')) } catch (err) { error.value = String(err) }
}
const canSave = computed(() => !props.loading && !saving.value && props.columns.length > 0 && !rowValueError(props.columns, fields))
function close(): void { if (!saving.value) emit('update:show', false) }
async function save(): Promise<void> {
 if (!canSave.value) return
 const invalid = rowJSONError(props.columns, fields, props.mode === 'edit' ? original : undefined)
 if (invalid) { error.value = text.value.jsonError.replace('{column}', invalid); return }
 const values = rowPayload(props.columns, fields, props.mode === 'edit' ? original : undefined)
 if (props.mode === 'edit' && !Object.keys(values).length) { close(); return }
 saving.value = true
 try {
  await api(props.mode === 'insert' ? 'POST' : 'PATCH', props.rowsPath, props.mode === 'insert' ? values : { where: props.rowPk, values })
  emit('saved'); emit('update:show', false)
 } catch (err) { error.value = err instanceof Error ? err.message : String(err); notification.error({ content: error.value, duration: 5000 }) }
 finally { saving.value = false }
}
type ParentOption = { label: string; value: string | number }
const parentOptions = ref<Record<string, ParentOption[]>>({})
const parentErrors = reactive<Record<string, boolean>>({})
const parentLoading = reactive<Record<string, boolean>>({})
const parentLabelColumns = reactive<Record<string, string>>({})
const parentSearchModes = reactive<Record<string, 'label' | 'key'>>({})
const parentSearchTerms: Record<string, string> = {}
const initialParentLoads: Record<string, Promise<void>> = {}
const requests: Record<string, number> = {}
const timers: Record<string, ReturnType<typeof setTimeout>> = {}
let generation = 0
function singleFK(name: string): ForeignKey | undefined { return props.foreignKeys?.find(fk => fk.columns.length === 1 && fk.refColumns.length === 1 && fk.columns[0] === name) }
function parentPath(fk: ForeignKey): string {
 const [base, query = ''] = props.rowsPath.split('?')
 let prefix = base!.slice(0, base!.indexOf('/tables/'))
 const params = new URLSearchParams(query)
 if (fk.refSchema) { if (catalog.schemaAware) params.set('schema', fk.refSchema); else { prefix = `/api/databases/${encodeURIComponent(fk.refSchema)}`; params.delete('schema') } }
 return `${prefix}/tables/${encodeURIComponent(fk.refTable)}/rows?${params.toString()}`
}
function relatedHref(fk: ForeignKey): string {
 const target = new URL(parentPath(fk), 'http://local')
 const db = decodeURIComponent(target.pathname.split('/')[3]!)
 const query = target.searchParams.toString()
 return `/db/${encodeURIComponent(db)}/table/${encodeURIComponent(fk.refTable)}${query ? '?' + query : ''}`
}
function fkOptions(name: string): ParentOption[] {
 const options = [...(parentOptions.value[name] ?? [])]
 const current = fields[name]?.value
 if (current !== null && current !== undefined && current !== '' && typeof current !== 'boolean' && !options.some(option => option.value === current)) options.unshift({ label: String(current), value: current })
 return options
}
async function loadParent(name: string, search = ''): Promise<void> {
 const fk = singleFK(name)
 if (!fk || !props.show) return
 const request = (requests[name] ?? 0) + 1; requests[name] = request
 const opened = generation
 parentLoading[name] = true
 const target = new URL(parentPath(fk), 'http://local')
 target.searchParams.set('limit', '200'); target.searchParams.set('offset', '0')
 const labelColumn = parentLabelColumns[name]
 if (search) target.searchParams.append('filter', labelColumn && parentSearchModes[name] !== 'key' ? `${labelColumn}:CONTAINS:${search}` : `${fk.refColumns[0]}:=:${search}`)
 try {
  const page = await api<Page>('GET', target.pathname + target.search)
  if (generation !== opened) return
  const keyIndex = page.columns.findIndex(col => col.name === fk.refColumns[0])
  const textColumn = (col: Page['columns'][number], index: number) => index !== keyIndex && !col.type.endsWith('[]') && /char|text/i.test(col.type)
  let labelIndex = page.columns.findIndex((col, index) => textColumn(col, index) && /^(name|title|label)$/i.test(col.name))
  if (labelIndex < 0) labelIndex = page.columns.findIndex(textColumn)
  if (labelIndex >= 0) parentLabelColumns[name] = page.columns[labelIndex]!.name
  // An obsolete initial page may still supply metadata for the latest queued search.
  if (requests[name] !== request) return
  parentOptions.value[name] = keyIndex < 0 ? [] : page.rows.flatMap(row => {
   const value = row[keyIndex]
   if (value === null || value === undefined) return []
   const label = labelIndex >= 0 ? row[labelIndex] : undefined
   return [{ label: label == null || label === '' ? String(value) : `${label} (${value})`, value: typeof value === 'number' ? value : String(value) }]
  })
  parentErrors[name] = false
 } catch { if (generation === opened && requests[name] === request) parentErrors[name] = true }
 finally { if (generation === opened && requests[name] === request) parentLoading[name] = false }
}
function searchParent(name: string, search: string): void {
 parentSearchTerms[name] = search
 clearTimeout(timers[name]); requests[name] = (requests[name] ?? 0) + 1
 const request = requests[name]; const opened = generation
 timers[name] = setTimeout(async () => {
  await initialParentLoads[name]
  if (opened !== generation || requests[name] !== request) return
  void loadParent(name, search)
 }, 250)
}
function setParentSearchMode(name: string, mode: 'label' | 'key'): void {
 parentSearchModes[name] = mode
 searchParent(name, parentSearchTerms[name] ?? '')
}
watch(() => [props.show, props.foreignKeys, props.rowsPath] as const, () => {
 generation++; parentOptions.value = {}
 for (const name of Object.keys(parentLabelColumns)) delete parentLabelColumns[name]
 for (const name of Object.keys(parentSearchModes)) delete parentSearchModes[name]
 for (const name of Object.keys(parentSearchTerms)) delete parentSearchTerms[name]
 for (const timer of Object.values(timers)) clearTimeout(timer)
 if (props.show) for (const fk of props.foreignKeys ?? []) if (fk.columns.length === 1) initialParentLoads[fk.columns[0]!] = loadParent(fk.columns[0]!)
}, { immediate: true, deep: true })
onBeforeUnmount(() => { generation++; for (const timer of Object.values(timers)) clearTimeout(timer) })
</script>
<style scoped>
.row-modal__fields { display: flex; flex-direction: column; gap: 12px; }
.row-modal__field { display: grid; grid-template-columns: minmax(90px, 130px) minmax(0, 1fr) auto; align-items: start; gap: 8px; }
.row-modal__label { overflow-wrap: anywhere; }
.row-modal__control { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.row-modal__input { width: 100%; }
.row-modal__choices { display: flex; flex-direction: column; gap: 4px; max-width: 170px; }
.row-modal__required { font-size: 12px; opacity: .6; }
.row-modal__related { display: flex; flex-direction: column; }
.row-modal__error { color: var(--error-color, #c33); overflow-wrap: anywhere; }
.row-modal__date { box-sizing: border-box; padding: 5px; background: transparent; color: inherit; border: 1px solid #888; }
.row-modal__footer { display: flex; justify-content: flex-end; gap: 8px; }
@media (max-width: 600px) { .row-modal__field { grid-template-columns: 1fr; } .row-modal__choices { max-width: none; } }
</style>
