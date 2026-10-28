<template>
  <section class="foreign-key-editor">
    <header><h3>{{ t('foreignKeys.title') }}</h3><button data-add-fk :disabled="saving" @click="addKey">{{ t('foreignKeys.add') }}</button></header>
    <p v-if="!rows.length">{{ t('foreignKeys.empty') }}</p>
    <fieldset v-for="row in rows" :key="row.key" :disabled="saving">
      <div class="fields"><label>{{ t('foreignKeys.name') }}<input v-model="row.name" data-fk-name></label><label>{{ t('foreignKeys.schema') }}<input v-model="row.refSchema" data-ref-schema @change="row.referenceColumns = []"></label><label>{{ t('foreignKeys.table') }}<input v-model="row.refTable" data-ref-table :list="`fk-tables-${row.key}`" @change="row.referenceColumns = []"><datalist :id="`fk-tables-${row.key}`"><option v-for="item in catalog.tables.filter(v => !v.isView)" :key="item.name" :value="item.name"></option></datalist></label><button data-load-reference :disabled="!row.refTable || row.loading" @click="loadReference(row)">{{ t('foreignKeys.loadColumns') }}</button><button @click="rows = rows.filter(v => v !== row)">{{ t('foreignKeys.remove') }}</button></div>
      <p v-if="row.referenceError" role="alert">{{ row.referenceError }}</p>
      <div v-for="(pair, index) in row.pairs" :key="index" class="mapping"><label>{{ t('foreignKeys.local') }}<select v-model="pair.local" data-local-column><option value=""></option><option v-for="column in columnNames" :key="column" :value="column">{{ column }}</option></select></label><span aria-hidden="true">→</span><label>{{ t('foreignKeys.remote') }}<select v-if="row.referenceColumns.length" v-model="pair.remote" data-remote-column><option value=""></option><option v-for="column in row.referenceColumns" :key="column" :value="column">{{ column }}</option></select><input v-else v-model="pair.remote" data-remote-column></label><button @click="row.pairs.splice(index, 1)">{{ t('foreignKeys.remove') }}</button></div>
      <button data-add-mapping @click="row.pairs.push({ local: '', remote: '' })">{{ t('foreignKeys.addMapping') }}</button>
      <div class="fields"><label>{{ t('foreignKeys.onDelete') }}<select v-model="row.onDelete"><option value="">{{ t('foreignKeys.defaultAction') }}</option><option v-for="action in actions" :key="action" :value="action">{{ action }}</option></select></label><label>{{ t('foreignKeys.onUpdate') }}<select v-model="row.onUpdate"><option value="">{{ t('foreignKeys.defaultAction') }}</option><option v-for="action in actions" :key="action" :value="action">{{ action }}</option></select></label></div>
    </fieldset>
    <p class="hint">{{ t('foreignKeys.hint') }}</p><p v-if="error" role="alert">{{ error }}</p><p v-if="notice" role="status">{{ notice }}</p>
    <button v-if="table !== '_new'" data-save-fks :disabled="saving || !valid" @click="save">{{ t('foreignKeys.save') }}</button>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { api } from '../../api/client'
import type { ForeignKey, TableDef } from '../../api/types'
import { withSchema } from '../../lib/schema'
import { useCatalogStore } from '../../stores/catalog'

const props = defineProps<{ db: string; table: string; modelValue: ForeignKey[]; columnNames: string[] }>()
const emit = defineEmits<{ 'update:modelValue': [value: ForeignKey[]]; saved: [] }>()
const { t } = useI18n(), catalog = useCatalogStore()
interface EditableKey { key: number; name: string; refSchema: string; refTable: string; pairs: { local: string; remote: string }[]; onDelete: string; onUpdate: string; referenceColumns: string[]; referenceError: string; loading: boolean; request: number }
const rows = ref<EditableKey[]>([]), saving = ref(false), error = ref(''), notice = ref('')
let keySequence = 0
const actions = computed(() => ['NO ACTION', 'RESTRICT', 'CASCADE', 'SET NULL', ...(catalog.schemaAware ? ['SET DEFAULT'] : [])])
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
async function loadReference(row: EditableKey) {
 const current = ++row.request, schema = row.refSchema || catalog.schema || 'public', db = catalog.schemaAware ? props.db : row.refSchema || props.db
 const table = row.refTable
 row.loading = true; row.referenceError = ''
 try { const body = await api<TableDef>('GET', withSchema(`/api/databases/${encodeURIComponent(db)}/tables/${encodeURIComponent(table)}`, schema, catalog.schemaAware)); if (current === row.request && table === row.refTable) row.referenceColumns = body.columns.map(column => column.name) }
 catch (err) { if (current === row.request) row.referenceError = err instanceof Error ? err.message : String(err) }
 finally { if (current === row.request) row.loading = false }
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
header, .fields, .mapping { display: flex; align-items: center; flex-wrap: wrap; gap: .7rem; } header { justify-content: space-between; } fieldset { margin: .8rem 0; border: 1px solid #8885; border-radius: 4px; padding: 1rem; } label { display: flex; flex-direction: column; gap: .4rem; margin: .5rem 0; } button, input, select { font: inherit; color: inherit; background: transparent; border: 1px solid #8886; border-radius: 4px; padding: .4rem .6rem; } button { cursor: pointer; } button:disabled { opacity: .5; cursor: default; } .hint { font-size: .9em; opacity: .7; } [role='alert'] { color: #d03050; }
</style>
