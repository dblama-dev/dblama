<template>
  <section class="table-view">
    <n-breadcrumb>
      <n-breadcrumb-item @click="router.push('/server')">{{ t('server.title') }}</n-breadcrumb-item><n-breadcrumb-item @click="router.push(catalog.path(`/db/${encodeURIComponent(db)}`))">{{ db }}</n-breadcrumb-item>
      <n-breadcrumb-item v-if="catalog.schema" @click="router.push(catalog.path(`/db/${encodeURIComponent(db)}`))">{{ catalog.schema }}</n-breadcrumb-item>
      <n-breadcrumb-item>{{ isNewTable ? t('table.new') : table }}</n-breadcrumb-item>
    </n-breadcrumb>
    <n-tabs type="line" :value="view" @update:value="switchView">
      <n-tab name="data">{{ t('table.data') }}</n-tab>
      <n-tab name="structure">{{ t('table.structure') }}</n-tab>
      <n-tab v-if="!isNewTable" name="sql">{{ t('navigationWorkflows.ddl') }}</n-tab>
      <n-tab v-if="!isNewTable" name="operations">{{ t('navigationWorkflows.operations') }}</n-tab>
    </n-tabs>
    <nav v-if="!isNewTable" class="table-view__actions" aria-label="Table actions">
      <n-button size="small" @click="openInsert">{{ t('navigationWorkflows.insert') }}</n-button>
      <n-button v-for="action in ['console','search','import','export']" :key="action" size="small" @click="router.push(tableContextLink(db,table,action,catalog.schema))">{{ t(`navigationWorkflows.${action}`) }}</n-button>
      <n-button data-testid="table-partitions" v-if="session.sessionInfo?.caps.partitions" size="small" @click="router.push(catalog.path(`/db/${encodeURIComponent(db)}/table/${encodeURIComponent(table)}/partitions`))">{{ t('navigationWorkflows.partitions') }}</n-button>
    </nav>
    <!-- Create mode (_new): no rows exist yet, the data tab is a
         placeholder and only the structure editor is usable. -->
    <div v-if="view === 'data' && isNewTable" class="table-view__placeholder">
      {{ t('table.newDataHint') }}
    </div>
    <DataGrid
      v-else-if="view === 'data'"
      :columns="page?.columns ?? []"
      :rows="page?.rows ?? []"
      :total-rows="page?.totalRows ?? 0"
      :loading="loading"
      :page-index="pageIndex"
      :page-size="pageSize"
      :pk-columns="pkColumns"
      :sort="sort"
      :filters="filters"
      :table-name="`${db}.${table}`"
      @update:page="onPage"
      @update:page-size="onPageSize"
      @update:sort="onSort"
      @apply-filters="onFilters"
      @refresh="refetch"
      @add-row="onAddRow"
      @edit-row="onEditRow"
      @edit-rows="onEditRows"
      @copy-row="onCopyRow"
      @copy-rows="copyRows"
      @export-selection="exportSelection"
      @delete-row="deleteRows([$event])"
      @delete-rows="deleteRows"
    />
    <StructureEditor
      v-else-if="view === 'structure'"
      :db="db"
      :table="table"
      @renamed="onTableRenamed"
      @created="onTableCreated"
    />
    <TableOperations v-else-if="view === 'operations'" :db="db" :table="table" @changed="refetch" @dropped="router.push(catalog.path(`/db/${encodeURIComponent(db)}`))" />
    <TableSQL v-else-if="view === 'sql'" :db="db" :table="table" />
    <BulkUpsert v-if="!isNewTable && view === 'data' && session.sessionInfo?.caps.upsert" :path="catalog.path(rowsPath + '/upsert')" @saved="refetch" />
    <BulkEdit :show="bulkShow" :rows-path="catalog.path(rowsPath + '/rows')" :columns="modalColumns" :row-keys="bulkKeys" @update:show="bulkShow = $event" @saved="refetch" />
    <RowModal
      v-if="!isNewTable"
      :show="rowModal.show"
      :mode="rowModal.mode"
      :rows-path="catalog.path(rowsPath + '/rows')"
      :columns="modalColumns"
      :row="rowModal.row"
      :row-pk="rowModal.pk"
      :foreign-keys="defForeignKeys"
      @update:show="rowModal.show = $event"
      @saved="onRowSaved"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useNotification } from 'naive-ui'
import { NBreadcrumb, NBreadcrumbItem, NButton, NTab, NTabs } from 'naive-ui'

import { api, ApiError } from '../api/client'
import type { ColumnDef, ForeignKey, Page, TableDef } from '../api/types'
import { useCatalogStore } from '../stores/catalog'
import BulkEdit from '../components/BulkEdit.vue'
import TableOperations from '../components/TableOperations.vue'
import BulkUpsert from '../components/BulkUpsert.vue'
import { useSessionStore } from '../stores/session'
import DataGrid from '../components/DataGrid.vue'
import RowModal from '../components/RowModal.vue'
import StructureEditor from './structure/StructureEditor.vue'
import TableSQL from './structure/TableSQL.vue'
import { downloadExport } from '../lib/download'
import { buildRowsQuery, readTableRouteQuery, writeTableRouteQuery, tableContextLink, type RowsFilter, type RowsSort } from '../lib/tableQuery'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const notification = useNotification()
const catalog = useCatalogStore()
const session = useSessionStore()

// vue-router hands the path params over already decoded; re-encode them
// for the api path (identifiers may contain %, spaces, ...).
const db = computed(() => String(route.params.db ?? ''))
const table = computed(() => String(route.params.t ?? ''))
const view = computed(() => {
  if (route.query.view === 'sql') return isNewTable.value ? 'structure' : 'sql'
  if (route.query.view === 'operations' && !isNewTable.value) return 'operations'
  return route.query.view === 'structure' ? 'structure' : 'data'
})
// The literal "_new" is the create-table placeholder, not a table name.
const isNewTable = computed(() => table.value === '_new')

function switchView(next: string | number): void {
  void router.push({ query: { ...route.query, view: next } })
}

// After a structure-editor rename the route must follow the new table
// name; the db/table watches then reload both views' data.
async function onTableRenamed(next: string): Promise<void> {
  await router.replace({
    name: route.name,
    params: { ...route.params, t: next },
    query: route.query,
  })
}

// After a create-mode save: refresh the tree with the new table and move
// the route onto it (structure view stays open for further editing).
async function onTableCreated(next: string): Promise<void> {
  await useCatalogStore().invalidate(db.value)
  await router.replace({
    name: route.name,
    params: { ...route.params, t: next },
    query: { ...route.query, view: 'structure' },
  })
}

const rowsPath = computed(
  () => `/api/databases/${encodeURIComponent(db.value)}/tables/${encodeURIComponent(table.value)}`,
)
const pkPath = computed(() => catalog.path(`${rowsPath.value}/pk`))

// All fetch logic for the data view lives here: DataGrid is presentational
// and reports query-state changes back as events.
const pkColumns = ref<string[]>([])
const page = ref<Page | null>(null)
const loading = ref(false)
const initialQuery = readTableRouteQuery(route.query)
const pageIndex = ref(initialQuery.page)
const pageSize = ref(initialQuery.pageSize)
const sort = ref<RowsSort | null>(initialQuery.sort)
const filters = ref<RowsFilter[]>(initialQuery.filters)
function restoreQuery(): void {
  const state = readTableRouteQuery(route.query)
  pageIndex.value=state.page; pageSize.value=state.pageSize; sort.value=state.sort; filters.value=state.filters
}
async function persistQuery(): Promise<void> {
  await router.push({query:writeTableRouteQuery(route.query,{page:pageIndex.value,pageSize:pageSize.value,sort:sort.value,filters:filters.value})})
}
watch([() => route.query.filters,() => route.query.sort,() => route.query.page,() => route.query.pageSize], () => {
 const before=JSON.stringify(queryState());restoreQuery()
 if(JSON.stringify(queryState())!==before)void refetch()
})

function queryState() {
  return {
    limit: pageSize.value,
    offset: (pageIndex.value - 1) * pageSize.value,
    sort: sort.value,
    filters: filters.value,
  }
}

function notifyError(err: unknown): void {
  const message = err instanceof ApiError ? err.message : String(err)
  notification.error({ content: message, duration: 5000 })
}

// Rows and primary-key metadata have separate request sequences. A filter
// request must not invalidate the key metadata still loading for this table.
let fetchSeq = 0
let pkFetchSeq = 0
let reloadSeq = 0
async function loadPk(): Promise<void> {
  const seq = ++pkFetchSeq
  const path = pkPath.value
  try {
    const pk = await api<{ columns: string[] }>('GET', path)
    if (seq !== pkFetchSeq || path !== pkPath.value) return
    pkColumns.value = pk.columns
  } catch (err) {
    if (seq !== pkFetchSeq || path !== pkPath.value) return
    notifyError(err)
  }
}

async function refetch(): Promise<void> {
  const seq = ++fetchSeq
  loading.value = true
  try {
    const next = await api<Page>('GET', catalog.path(`${rowsPath.value}/rows?${buildRowsQuery(queryState())}`))
    if (seq !== fetchSeq) return
    page.value = next
  } catch (err) {
    if (seq !== fetchSeq) return
    notifyError(err)
  } finally {
    if (seq === fetchSeq) {
      loading.value = false
    }
  }
}

async function reload(): Promise<void> {
  const seq=++reloadSeq
  ++fetchSeq
  ++pkFetchSeq
  // The _new placeholder has no table behind it — no pk or rows fetch.
  if (isNewTable.value) return
  rowModal.value.show = false
  page.value = null
  pkColumns.value = []
  restoreQuery()
  bulkShow.value = false
  defRequest++
  await loadPk()
  if(seq!==reloadSeq)return
  await refetch()
}

onMounted(() => {
  void reload()
})

watch([db, table, () => catalog.schema], () => {
  void reload()
})
watch(view,(next,previous)=>{if(next==='data' && previous!=='data')void reload()})

async function onPage(next: number): Promise<void> {
  pageIndex.value = next
  await persistQuery()
  await refetch()
}

async function onPageSize(size: number): Promise<void> {
  pageSize.value = size
  pageIndex.value = 1
  await persistQuery()
  await refetch()
}

async function onSort(next: RowsSort | null): Promise<void> {
  sort.value = next
  pageIndex.value = 1
  await persistQuery()
  await refetch()
}

async function onFilters(next: RowsFilter[]): Promise<void> {
  filters.value = next
  pageIndex.value = 1
  await persistQuery()
  await refetch()
}

async function deleteRows(pks: Record<string, unknown>[]): Promise<void> {
  if (pks.length === 0) return
  try {
    await api('DELETE', catalog.path(`${rowsPath.value}/rows`), { where: pks })
  } catch (err) {
    notifyError(err)
    return
  }
  await refetch()
  // A delete can empty the current page: step back one page if so.
  if ((page.value?.rows.length ?? 0) === 0 && pageIndex.value > 1) {
    pageIndex.value -= 1
    await persistQuery()
    await refetch()
  }
}

// --- Row insert/edit modal ----------------------------------------------

interface EditableRow extends Record<string, unknown> {
  __pk: Record<string, unknown> | null
}

interface RowModalState {
  show: boolean
  mode: 'insert' | 'edit'
  row: Record<string, unknown> | null
  pk: Record<string, unknown> | null
}

const rowModal = ref<RowModalState>({ show: false, mode: 'insert', row: null, pk: null })

// Richer column definitions (enum values, defaults) come from the TableDef
// endpoint; on failure the modal falls back to the page's ColumnMeta list.
const defColumns = ref<ColumnDef[] | null>(null)
const defForeignKeys = ref<ForeignKey[]>([])
const modalColumns = computed<ColumnDef[]>(() => (defColumns.value ?? page.value?.columns ?? []).filter(col => !('generated' in col && col.generated)))
let defRequest = 0

async function fetchDef(): Promise<boolean> {
  const request = ++defRequest
  const path = catalog.path(rowsPath.value)
  try {
    const def = await api<TableDef>('GET', path)
    if (request !== defRequest || path !== catalog.path(rowsPath.value)) return false
    defColumns.value = def.columns
    defForeignKeys.value = def.foreignKeys ?? []
    return true
  } catch (err) {
    if (request !== defRequest || path !== catalog.path(rowsPath.value)) return false
    notifyError(err)
    return false
  }
}

async function onAddRow(): Promise<void> {
  defColumns.value = null
  if (!await fetchDef()) return
  rowModal.value = { show: true, mode: 'insert', row: null, pk: null }
}

async function onEditRow(row: EditableRow): Promise<void> {
  const values: Record<string, unknown> = {}
  for (const meta of page.value?.columns ?? []) {
    values[meta.name] = row[meta.name]
  }
  defColumns.value = null
  if (!await fetchDef()) return
  rowModal.value = { show: true, mode: 'edit', row: values, pk: row.__pk }
}

const bulkShow = ref(false)
const bulkKeys = ref<Record<string,unknown>[]>([])
async function onEditRows(keys: Record<string,unknown>[]): Promise<void> {
  const captured = keys.map(key => ({...key}))
  if (!await fetchDef()) return
  bulkKeys.value=captured; bulkShow.value=true
}
async function openInsert(): Promise<void> {
  if(view.value!=='data'){
    const failure=await router.push({query:{...route.query,view:'data'}})
    if(failure)return
  }
  await onAddRow()
}

async function onCopyRow(row: EditableRow): Promise<void> {
  const sourcePath = catalog.path(rowsPath.value)
  if (!await fetchDef() || catalog.path(rowsPath.value) !== sourcePath) return
  const columns = defColumns.value ?? page.value?.columns ?? []
  const values: Record<string, unknown> = {}
  for (const col of columns) {
    if (('autoIncrement' in col && col.autoIncrement) || ('generated' in col && col.generated)) continue
    values[col.name] = row[col.name]
  }
  rowModal.value = { show: true, mode: 'insert', row: values, pk: null }
}

async function copyRows(where: Record<string,unknown>[]): Promise<void> {
  try { await api('POST',catalog.path(`${rowsPath.value}/copy-rows`),{where});await refetch() }
  catch(err) {notifyError(err)}
}

async function exportSelection(): Promise<void> {
  try {
    await downloadExport(db.value, {
      format: 'csv', tables: [table.value],
      query: { filters: filters.value.map(f => ({column:f.column,operator:f.op,value:f.value})), sorts: sort.value ? [sort.value] : [] },
    }, catalog.schemaAware ? catalog.schema : null)
  } catch (err) { notifyError(err) }
}

function onRowSaved(): void {
  rowModal.value = { ...rowModal.value, show: false }
  void refetch()
}
</script>

<style scoped>
.table-view {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.table-view__actions { display:flex;flex-wrap:wrap;gap:6px; }
.table-view__placeholder {
  padding: 24px;
  opacity: 0.6;
}
</style>
