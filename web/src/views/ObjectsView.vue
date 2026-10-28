<template>
  <section class="objects-view">
    <header><h2>{{ t(`objects.${kind}`) }}</h2><button :disabled="busy || !supported" @click="load">{{ t('objects.refresh') }}</button></header>
    <p v-if="!supported" role="status">{{ t('objects.unsupported') }}</p>
    <template v-else>
      <p v-if="error" role="alert" class="error">{{ error }}</p><p v-if="notice" role="status">{{ notice }}</p>
      <template v-if="kind === 'partitions'">
        <label>{{ t('objects.table') }}<select v-model="table"><option value="">{{ t('objects.selectTable') }}</option><option v-for="item in catalog.tables.filter(v => !v.isView)" :key="item.name" :value="item.name">{{ item.name }}</option></select></label>
        <table v-if="table" class="catalog-table"><thead><tr><th>{{ t('objects.name') }}</th><th>{{ t('objects.method') }}</th><th>{{ t('objects.expression') }}</th><th>{{ t('objects.boundary') }}</th><th>{{ t('objects.rows') }}</th><th></th></tr></thead><tbody><tr v-for="part in partitions" :key="part.name"><td>{{ part.name }}</td><td>{{ part.method }}</td><td>{{ part.expression }}</td><td>{{ part.description }}</td><td>{{ part.rows }}</td><td class="actions"><button :disabled="busy" @click="scheduleOperation('truncate', part.name)">{{ t('objects.truncate') }}</button><button :data-partition-drop="part.name" :disabled="busy" @click="scheduleOperation('partition-drop', part.name)">{{ t('objects.remove') }}</button></td></tr></tbody></table>
        <label>{{ t('objects.partitionSource') }}<textarea v-model="source" data-source rows="4" :disabled="busy"></textarea></label><p class="hint">{{ t('objects.partitionHint') }}</p><button data-save :disabled="busy || !table || !source.trim()" @click="save">{{ t('objects.addPartition') }}</button>
      </template>
      <div v-else class="object-workspace">
        <aside><input v-model="filter" :aria-label="t('objects.search')" :placeholder="t('objects.search')"><button :disabled="busy" @click="newObject">{{ t('objects.create') }}</button><p v-if="!objects.length">{{ t('objects.empty') }}</p><ul><li v-for="item in filteredObjects" :key="`${item.name}|${item.identity}|${item.table}`"><button :data-object="item.name" :class="{ active: selected === item }" :disabled="busy" @click="selectObject(item)">{{ item.name }}<small>{{ item.kind }} {{ item.identity }} {{ item.table }}</small></button></li></ul></aside>
        <article v-if="editing"><div class="fields"><label>{{ t('objects.name') }}<input v-model="name" :disabled="busy || !!selected" data-name></label><label v-if="kind === 'routines'">{{ t('objects.kind') }}<select v-model="routineKind" :disabled="busy || !!selected"><option value="FUNCTION">FUNCTION</option><option v-if="session.sessionInfo?.caps.procedures" value="PROCEDURE">PROCEDURE</option></select></label><label v-if="kind === 'triggers'">{{ t('objects.table') }}<input v-model="objectTable" :disabled="busy"></label></div>
          <label>{{ t('objects.source') }}<textarea v-model="source" data-source rows="18" spellcheck="false" :disabled="busy"></textarea></label><p class="hint">{{ t('objects.sourceHint') }}</p><div class="actions"><button data-save :disabled="busy || !name || !source.trim()" @click="save">{{ t('objects.save') }}</button><button v-if="selected" data-delete :disabled="busy" @click="scheduleOperation('object-drop', name)">{{ t('objects.remove') }}</button></div>
          <div v-if="selected?.kind === 'PROCEDURE' && session.sessionInfo?.caps.procedures" class="procedure-call"><label>{{ t('objects.parameters') }}<textarea v-model="argumentsJSON" data-arguments rows="3" :disabled="busy"></textarea></label><p class="hint">{{ t('objects.parameterHint') }}</p><button data-call :disabled="busy" @click="call">{{ t('objects.call') }}</button><ResultGrid v-for="(result, index) in results" :key="index" :result="result" :index="index + 1" /></div>
        </article><p v-else>{{ t('objects.select') }}</p>
      </div>
      <div v-if="pending" role="alertdialog" aria-modal="true" :aria-label="t('objects.confirm')" class="confirmation"><p>{{ t(pending.operation === 'object-drop' ? 'objects.deleteConfirm' : 'objects.partitionConfirm') }} <strong>{{ pending.label }}</strong></p><button :disabled="busy" @click="pending = null">{{ t('objects.cancel') }}</button><button :disabled="busy" data-confirm @click="confirmOperation">{{ t('objects.confirm') }}</button></div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { api } from '../api/client'
import type { QueryResult } from '../api/types'
import ResultGrid from '../components/ResultGrid.vue'
import { useCatalogStore } from '../stores/catalog'
import { useSessionStore } from '../stores/session'

interface DbObject { name: string; kind: string; source: string; table?: string; identity?: string }
interface Partition { name: string; method: string; expression: string; description: string; rows: number }
const { t } = useI18n()
const route = useRoute(), session = useSessionStore(), catalog = useCatalogStore()
const kind = computed(() => route.params.t ? 'partitions' : String(route.params.kind))
const supported = computed(() => ['views', 'triggers', 'routines', 'events', 'partitions'].includes(kind.value) && Boolean(session.sessionInfo?.caps[kind.value as keyof typeof session.sessionInfo.caps]))
const db = computed(() => String(route.params.db ?? ''))
const table = ref(String(route.params.t ?? catalog.tables.find(v => !v.isView)?.name ?? ''))
const base = computed(() => `/api/databases/${encodeURIComponent(db.value)}/${kind.value === 'partitions' ? `tables/${encodeURIComponent(table.value)}/partitions` : kind.value}`)
const objects = ref<DbObject[]>([]), partitions = ref<Partition[]>([]), selected = ref<DbObject | null>(null)
const editing = ref(false), busy = ref(false), error = ref(''), notice = ref(''), filter = ref(''), name = ref(''), source = ref(''), objectTable = ref(''), routineKind = ref('FUNCTION'), argumentsJSON = ref('[]'), results = ref<QueryResult[]>([])
type PendingOperation = { operation: 'object-drop' | 'partition-drop' | 'truncate'; path: string; label: string }
const pending = ref<PendingOperation | null>(null)
const filteredObjects = computed(() => objects.value.filter(o => o.name.toLowerCase().includes(filter.value.toLowerCase())))
let request = 0
function endpoint(suffix = '') { return catalog.path(base.value + suffix) }
function newObject() { selected.value = null; editing.value = true; name.value = ''; source.value = ''; objectTable.value = ''; routineKind.value = 'FUNCTION'; results.value = []; notice.value = ''; error.value = '' }
function selectObject(item: DbObject) { selected.value = item; editing.value = true; name.value = item.name; source.value = item.source; objectTable.value = item.table ?? ''; routineKind.value = item.kind; results.value = []; notice.value = ''; error.value = '' }
async function load() {
 const current = ++request
 objects.value = []; partitions.value = []; selected.value = null; editing.value = false; pending.value = null; results.value = []; source.value = ''; error.value = ''
 if (!supported.value || (kind.value === 'partitions' && !table.value)) { busy.value = false; return }
 busy.value = true
 try { const body = await api<{ objects?: DbObject[]; partitions?: Partition[] }>('GET', endpoint()); if (current !== request) return; objects.value = body.objects ?? []; partitions.value = body.partitions ?? [] }
 catch (err) { if (current === request) error.value = err instanceof Error ? err.message : String(err) }
 finally { if (current === request) busy.value = false }
}
async function mutate(action: () => Promise<unknown>, message: string) {
 if (busy.value) return
 busy.value = true; error.value = ''; notice.value = ''; const current = request
 try { await action(); if (current !== request) return; await load(); notice.value = t(message) }
 catch (err) { if (current === request) error.value = err instanceof Error ? err.message : String(err) }
 finally { if (current === request) busy.value = false }
}
function save() {
 const path = endpoint(kind.value === 'partitions' ? '' : `/${encodeURIComponent(name.value)}`)
 const payload = { source: source.value, kind: routineKind.value, table: objectTable.value, identity: selected.value?.identity ?? '' }
 void mutate(() => api(kind.value === 'partitions' ? 'POST' : 'PUT', path, payload), 'objects.saved')
}
function scheduleOperation(operation: PendingOperation['operation'], targetName: string) {
 let suffix = `/${encodeURIComponent(targetName)}`
 let label = targetName
 if (operation === 'truncate') {
  suffix += '/truncate'
  label += ` (${table.value})`
 }
 else if (operation === 'object-drop') {
  const object = selected.value
  if (!object) return
  suffix += `?${new URLSearchParams({ kind: object.kind, table: object.table ?? '', identity: object.identity ?? '' })}`
  label += [object.table, object.identity].filter(Boolean).map(value => ` (${value})`).join('')
 } else {
  label += ` (${table.value})`
 }
 pending.value = { operation, path: endpoint(suffix), label }
}
function confirmOperation() {
 const operation = pending.value; if (!operation) return
 void mutate(() => api(operation.operation === 'truncate' ? 'POST' : 'DELETE', operation.path), 'objects.deleted')
}
async function call() {
 let args: unknown
 try { args = JSON.parse(argumentsJSON.value); if (!Array.isArray(args) || args.some(v => v !== null && !['string', 'number', 'boolean'].includes(typeof v))) throw new Error() }
 catch { error.value = t('objects.invalidArgs'); return }
 const path = endpoint(`/${encodeURIComponent(name.value)}/call`), identity = selected.value?.identity ?? '', current = request
 busy.value = true; error.value = ''; results.value = []
 try { const body = await api<{ results: QueryResult[] }>('POST', path, { args, identity }); if (current === request) results.value = body.results }
 catch (err) { if (current === request) error.value = err instanceof Error ? err.message : String(err) }
 finally { if (current === request) busy.value = false }
}
watch(() => [db.value, kind.value, catalog.schema, table.value], () => { void load() }, { immediate: true })
watch(() => route.params.t, value => { if (value) table.value = String(value) })
</script>

<style scoped>
.objects-view { padding: 1rem; } header, .actions, .fields { display: flex; align-items: center; gap: .7rem; flex-wrap: wrap; } header { justify-content: space-between; } h2 { margin: 0 0 1rem; } button, input, select, textarea { font: inherit; color: inherit; border: 1px solid #8886; border-radius: 4px; background: transparent; padding: .5rem .7rem; } button { cursor: pointer; } button:disabled { cursor: default; opacity: .5; } label { display: flex; flex-direction: column; gap: .4rem; margin: .7rem 0; } textarea { width: 100%; box-sizing: border-box; font-family: monospace; resize: vertical; } .object-workspace { display: grid; grid-template-columns: minmax(180px, 25%) 1fr; gap: 1.5rem; } aside input { width: 100%; box-sizing: border-box; margin-bottom: .5rem; } aside ul { list-style: none; padding: 0; } aside li button { width: 100%; text-align: left; margin-bottom: .3rem; } small { display: block; opacity: .65; overflow-wrap: anywhere; } .active { border-color: #18a058; } article { min-width: 0; } .hint { opacity: .7; font-size: .9em; } .error { color: #d03050; } .confirmation { border: 1px solid #d03050; padding: 1rem; margin-top: 1rem; } .confirmation button { margin-right: .5rem; } .catalog-table { width: 100%; border-collapse: collapse; margin: 1rem 0; } th, td { padding: .5rem; text-align: left; border-bottom: 1px solid #8884; } .procedure-call { margin-top: 1.5rem; } @media (max-width: 750px) { .object-workspace { grid-template-columns: 1fr; } }
</style>
