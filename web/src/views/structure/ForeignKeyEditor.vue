<template>
  <section class="foreign-key-editor">
    <header class="foreign-key-editor__heading">
      <h3>{{ t('foreignKeys.title') }}</h3>
      <UiButton data-add-fk size="small" :disabled="saving" @click="addKey">
        <UiIcon name="plus" :size="14" />{{ t('foreignKeys.add') }}
      </UiButton>
    </header>
    <p v-if="!rows.length" class="hint">{{ t('foreignKeys.empty') }}</p>
    <fieldset v-for="row in rows" :key="row.key" :disabled="saving" :aria-label="row.name || t('foreignKeys.title')">
      <div class="foreign-key-editor__identity">
        <label>{{ t('foreignKeys.name') }}<UiInput v-model:value="row.name" data-fk-name /></label>
        <UiButton size="small" quaternary :aria-label="`${t('foreignKeys.removeKey')} ${row.name}`"
          :title="t('foreignKeys.removeKey')" @click="rows = rows.filter(v => v !== row)">
          <UiIcon name="x" :size="15" />{{ t('foreignKeys.remove') }}
        </UiButton>
      </div>
      <div class="foreign-key-editor__reference">
        <label>{{ t('foreignKeys.schema') }}
          <UiInput v-model:value="row.refSchema" data-ref-schema @update:value="resetReference(row)" />
        </label>
        <label>{{ t('foreignKeys.table') }}
          <UiInput v-model:value="row.refTable" data-ref-table :list="`fk-tables-${row.key}`"
            @focus="loadSuggestions(row)" @update:value="resetReference(row)" />
          <datalist :id="`fk-tables-${row.key}`"><option v-for="name in suggestions(row)" :key="name" :value="name" /></datalist>
        </label>
        <UiButton size="small" data-load-reference :disabled="!row.refTable" :loading="row.loading" @click="loadReference(row)">
          {{ t('foreignKeys.loadColumns') }}
        </UiButton>
      </div>
      <p v-if="row.referenceError" role="alert">{{ row.referenceError }}</p>
      <div class="foreign-key-editor__mappings">
        <div v-for="(pair, index) in row.pairs" :key="index" class="foreign-key-editor__pair">
          <label>{{ t('foreignKeys.local') }}<UiSelect v-model:value="pair.local" :options="localOptions" data-local-column /></label>
          <UiIcon name="arrow-right" :size="15" class="foreign-key-editor__arrow" />
          <label>{{ t('foreignKeys.remote') }}
            <UiSelect v-if="row.referenceColumns.length" v-model:value="pair.remote" :options="selectableColumns(row.referenceColumns)" data-remote-column />
            <UiInput v-else v-model:value="pair.remote" data-remote-column />
          </label>
          <UiButton size="small" quaternary class="foreign-key-editor__remove-pair" :aria-label="t('foreignKeys.removePair', { n: index + 1 })"
            :title="t('foreignKeys.removePair', { n: index + 1 })" @click="row.pairs.splice(index, 1)"><UiIcon name="x" :size="15" /></UiButton>
        </div>
        <UiButton size="small" data-add-mapping @click="row.pairs.push({ local: '', remote: '' })">
          <UiIcon name="plus" :size="14" />{{ t('foreignKeys.addMapping') }}
        </UiButton>
      </div>
      <div class="foreign-key-editor__actions">
        <label>{{ t('foreignKeys.onDelete') }}<UiSelect v-model:value="row.onDelete" :options="actionOptions" /></label>
        <label>{{ t('foreignKeys.onUpdate') }}<UiSelect v-model:value="row.onUpdate" :options="actionOptions" /></label>
      </div>
    </fieldset>
    <p class="hint">{{ t('foreignKeys.hint') }}</p>
    <p v-if="error" role="alert">{{ error }}</p><p v-if="notice" role="status">{{ notice }}</p>
    <UiButton v-if="table !== '_new'" size="small" type="primary" data-save-fks :loading="saving" :disabled="!valid" @click="save">
      {{ t('foreignKeys.save') }}
    </UiButton>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { UiButton, UiInput, UiSelect } from '../../ui'
import UiIcon from '../../components/UiIcon.vue'
import { api } from '../../api/client'
import type { ForeignKey, TableDef, TableInfo } from '../../api/types'
import { withSchema } from '../../lib/schema'
import { useCatalogStore } from '../../stores/catalog'

const props = defineProps<{ db: string; table: string; modelValue: ForeignKey[]; columnNames: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: ForeignKey[]]; saved: [] }>()
const { t } = useI18n(), catalog = useCatalogStore()
interface EditableKey { key: number; name: string; refSchema: string; refTable: string; pairs: { local: string; remote: string }[]; onDelete: string; onUpdate: string; referenceColumns: string[]; referenceError: string; loading: boolean; request: number }
const rows = ref<EditableKey[]>([]), saving = ref(false), error = ref(''), notice = ref('')
let keySequence = 0
const actions = computed(() => ['NO ACTION', 'RESTRICT', 'CASCADE', 'SET NULL', ...(catalog.schemaAware ? ['SET DEFAULT'] : [])])
function columnOptions(names: string[]) { return names.map(name => ({ label: name, value: name })) }
function selectableColumns(names: string[]) { return [{ label: '—', value: '' }, ...columnOptions(names)] }
const localOptions = computed(() => selectableColumns(props.columnNames))
const actionOptions = computed(() => [{ label: t('foreignKeys.defaultAction'), value: '' }, ...columnOptions(actions.value)])
function toModel(): ForeignKey[] { return rows.value.map(row => ({ name: row.name, columns: row.pairs.map(p => p.local), ...(row.refSchema ? { refSchema: row.refSchema } : {}), refTable: row.refTable, refColumns: row.pairs.map(p => p.remote), ...(row.onDelete ? { onDelete: row.onDelete } : {}), ...(row.onUpdate ? { onUpdate: row.onUpdate } : {}) })) }
function fromModel(fk: ForeignKey): EditableKey {
 let schema = fk.refSchema ?? '', table = fk.refTable
 if (!catalog.schemaAware && !schema && table.includes('.')) { const dot = table.indexOf('.'); schema = table.slice(0, dot); table = table.slice(dot + 1) }
 return { key: ++keySequence, name: fk.name, refSchema: schema, refTable: table, pairs: fk.columns.map((local, index) => ({ local, remote: fk.refColumns[index] ?? '' })), onDelete: fk.onDelete ?? '', onUpdate: fk.onUpdate ?? '', referenceColumns: [], referenceError: '', loading: false, request: 0 }
}
watch(() => props.modelValue, next => { if (JSON.stringify(next ?? []) !== JSON.stringify(toModel())) rows.value = (next ?? []).map(fromModel) }, { immediate: true, deep: true })
watch(rows, () => { emit('update:modelValue', toModel()); notice.value = '' }, { deep: true })
function addKey() { rows.value.push(fromModel({ name: '', columns: [''], refSchema: catalog.schemaAware ? catalog.schema ?? 'public' : props.db, refTable: '', refColumns: [''] })) }
const valid = computed(() => {
 const names = new Set<string>()
 return rows.value.every(row => {
  if (!row.name.trim() || names.has(row.name) || !row.refTable.trim() || !row.pairs.length) return false
  names.add(row.name)
  return row.pairs.every(p => props.columnNames.includes(p.local) && p.remote.trim()) && new Set(row.pairs.map(p => p.local)).size === row.pairs.length && new Set(row.pairs.map(p => p.remote)).size === row.pairs.length
 })
})
const activeCatalogPath = computed(() => catalog.path(`/api/databases/${encodeURIComponent(props.db)}/tables`))
const externalTables = ref(new Map<string, string[]>())
const pendingLists = new Set<string>()
function referencePath(row: EditableKey, table?: string): string {
 const db = !catalog.schemaAware && row.refSchema ? row.refSchema : props.db
 const path = `/api/databases/${encodeURIComponent(db)}/tables` + (table ? `/${encodeURIComponent(table)}` : '')
 return withSchema(path, row.refSchema || catalog.schema, catalog.schemaAware)
}
function suggestions(row: EditableKey): string[] {
 const path = referencePath(row)
 if (path === activeCatalogPath.value) {
  return catalog.tablesPath === path ? catalog.tables.filter(item => !item.isView).map(item => item.name) : []
 }
 return externalTables.value.get(path) ?? []
}
async function loadSuggestions(row: EditableKey): Promise<void> {
 const path = referencePath(row)
 if (path === activeCatalogPath.value) {
  await catalog.ensureTables(props.db)
  return
 }
 if (externalTables.value.has(path) || pendingLists.has(path)) return
 pendingLists.add(path)
 try {
  const body = await api<{ tables: TableInfo[] }>('GET', path)
  externalTables.value.set(path, body.tables.filter(item => !item.isView).map(item => item.name))
 } catch { /* Catalog permissions are optional for manual reference entry. */ }
 finally { pendingLists.delete(path) }
}
function resetReference(row: EditableKey): void {
 row.request++
 row.loading = false
 row.referenceError = ''
 row.referenceColumns = []
}
async function loadReference(row: EditableKey) {
 const current = ++row.request
 const path = referencePath(row, row.refTable)
 row.loading = true; row.referenceError = ''
 try {
  const body = await api<TableDef>('GET', path)
  if (current === row.request && path === referencePath(row, row.refTable)) row.referenceColumns = body.columns.map(column => column.name)
 } catch (err) {
  if (current === row.request) row.referenceError = err instanceof Error ? err.message : String(err)
 } finally { if (current === row.request) row.loading = false }
}
async function save() {
 if (saving.value || !valid.value) return
 saving.value = true; error.value = ''; notice.value = ''
 try { await api('POST', catalog.path(`/api/databases/${encodeURIComponent(props.db)}/tables/${encodeURIComponent(props.table)}/foreign-keys`), toModel()); notice.value = t('foreignKeys.saved'); emit('saved') }
 catch (err) { error.value = err instanceof Error ? err.message : String(err) }
 finally { saving.value = false }
}
</script>

<style scoped>
.foreign-key-editor { display: grid; justify-items: start; gap: 12px; container-type: inline-size; }
.foreign-key-editor__heading { display: flex; flex-wrap: wrap; width: 100%; gap: 6px 10px; align-items: center; justify-content: space-between; }
h3 { margin: 0; font-size: 15px; font-weight: 600; }
fieldset { display: grid; gap: 14px; width: 100%; min-width: 0; margin: 0; padding: 14px; border: 1px solid var(--db-line); background: var(--db-sidebar); border-radius: 8px; }
label { display: grid; align-content: end; gap: 4px; min-width: 0; color: var(--db-muted); font-size: 12px; font-weight: 550; }
.foreign-key-editor__identity, .foreign-key-editor__reference, .foreign-key-editor__actions, .foreign-key-editor__pair { display: grid; gap: 10px; align-items: end; }
.foreign-key-editor__identity { grid-template-columns: minmax(0, 1fr) auto; }
.foreign-key-editor__identity label { max-width: 26rem; }
.foreign-key-editor__reference { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) auto; }
.foreign-key-editor__mappings { display: grid; justify-items: start; gap: 8px; padding-block: 12px; border-block: 1px solid var(--db-line); }
.foreign-key-editor__pair { width: 100%; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto; }
.foreign-key-editor__arrow { margin-block-end: 10px; color: var(--db-muted); }
.foreign-key-editor__actions { grid-template-columns: repeat(2, minmax(0, 1fr)); }
p { margin: 0; }
.hint { max-width: 82ch; font-size: 12px; color: var(--db-muted); line-height: 1.5; }
[role=alert] { color: var(--db-danger); overflow-wrap: anywhere; }
@container (max-width: 35rem) {
  .foreign-key-editor__reference, .foreign-key-editor__actions { grid-template-columns: minmax(0, 1fr); }
  .foreign-key-editor__reference .ui-button { justify-self: start; }
  .foreign-key-editor__pair { grid-template-columns: minmax(0, 1fr) auto; gap: 6px; }
  .foreign-key-editor__pair label { grid-column: 1; }
  .foreign-key-editor__arrow { display: none; }
  .foreign-key-editor__remove-pair { grid-row: 2; grid-column: 2; }
}
</style>
