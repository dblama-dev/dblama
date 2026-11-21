<template>
  <section class="table-view db-page">
    <header class="db-page-head table-view__head">
      <div class="db-heading">
        <span class="db-heading-icon"><UiIcon :name="catalogEntry?.isView ? 'eye' : 'table'" :size="24" /></span>
        <div class="table-view__identity">
          <h1 class="db-title">{{ isNewTable ? t('table.new') : table }}</h1>
          <p v-if="catalogEntry?.comment" class="db-secondary">{{ catalogEntry.comment }}</p>
        </div>
      </div>
      <div v-if="!isNewTable" class="table-view__actions">
        <ui-button :type="view === 'data' ? 'primary' : 'default'" data-testid="table-add-row" @click="openInsert">
          <template #icon><UiIcon name="plus" /></template>{{ t('grid.addRow') }}
        </ui-button>
        <details ref="toolsMenu" class="table-tools" @keydown.esc.stop="closeTools(true)" @focusout="toolsFocusOut">
          <summary><UiIcon name="more" :size="16" />{{ t('rowWorkflows.tableTools') }}</summary>
          <nav class="table-tools__list" :aria-label="t('rowWorkflows.tableTools')" @click="closeTools(false)">
            <button v-for="item in contextTools" :key="item.action" type="button"
              @click="router.push(tableContextLink(db, table, item.action, catalog.schema))">
              <UiIcon :name="item.icon" />{{ t('navigationWorkflows.' + item.action) }}
            </button>
            <button v-if="session.sessionInfo?.caps.partitions" type="button" data-testid="table-partitions"
              @click="router.push(catalog.path(`/db/${encodeURIComponent(db)}/table/${encodeURIComponent(table)}/partitions`))">
              <UiIcon name="schema" />{{ t('navigationWorkflows.partitions') }}
            </button>
            <button v-if="view === 'data' && session.sessionInfo?.caps.upsert" type="button" @click="upsertShow = true">
              <UiIcon name="code" />{{ t('grid.upsert') }}
            </button>
          </nav>
        </details>
      </div>
    </header>

    <nav class="table-view__tabs" :aria-label="t('tree.tables')">
      <button v-for="section in tableSections" :key="section.name" type="button" class="table-section"
        :data-section="section.name" :aria-current="view === section.name ? 'page' : undefined" @click="switchView(section.name)">
        <UiIcon :name="section.icon" />{{ t(section.label) }}
      </button>
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
      :statement="page?.statement"
      :loading="loading"
      :page-index="pageIndex"
      :page-size="pageSize"
      :pk-columns="pkColumns"
      :visible-columns="visibleColumns"
      :sort="sort"
      :filters="filters"
      :table-name="`${db}.${table}`"
      @update:page="onPage"
      @update:page-size="onPageSize"
      @update:sort="onSort"
      @update:visible-columns="onVisibleColumns"
      @apply-filters="onFilters"
      @refresh="refetch"
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
    <BulkUpsert v-if="!isNewTable && view === 'data' && session.sessionInfo?.caps.upsert" :key="catalog.path(rowsPath)"
      v-model:show="upsertShow" :path="catalog.path(rowsPath + '/upsert')" @saved="refetch" />
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
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useNotification } from '../ui'
import { UiButton } from '../ui'
import UiIcon from '../components/UiIcon.vue'

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
const tableSections = computed(() => [
  { name: 'data', icon: 'table', label: 'table.data' },
  { name: 'structure', icon: 'columns', label: 'table.structure' },
  ...(isNewTable.value ? [] : [
    { name: 'sql', icon: 'code', label: 'navigationWorkflows.ddl' },
    { name: 'operations', icon: 'settings', label: 'navigationWorkflows.operations' },
  ]),
])

const catalogPath = computed(() => catalog.path(`/api/databases/${encodeURIComponent(db.value)}/tables`))
watch([catalogPath, isNewTable], () => {
  if (!isNewTable.value) void catalog.ensureTables(db.value)
}, { immediate: true })
const catalogEntry = computed(() => catalog.tablesPath === catalogPath.value
  ? catalog.tables.find(entry => entry.name === table.value)
  : undefined)
const toolsMenu = ref<HTMLDetailsElement | null>(null)
const upsertShow = ref(false)
const contextTools = [
  { action: 'console', icon: 'terminal' },
  { action: 'search', icon: 'search' },
  { action: 'import', icon: 'upload' },
  { action: 'export', icon: 'download' },
] as const
function closeTools(focusTrigger: boolean): void {
  if (!toolsMenu.value?.open) return
  toolsMenu.value.open = false
  if (focusTrigger) toolsMenu.value.querySelector('summary')?.focus()
}
function toolsFocusOut(event: FocusEvent): void {
  if (event.relatedTarget instanceof Node && !toolsMenu.value?.contains(event.relatedTarget)) closeTools(false)
}
function pointerOutside(event: PointerEvent): void {
  if (event.target instanceof Node && !toolsMenu.value?.contains(event.target)) closeTools(false)
}
onMounted(() => document.addEventListener('pointerdown', pointerOutside))
onBeforeUnmount(() => document.removeEventListener('pointerdown', pointerOutside))
watch([db, table, () => catalog.schema, view], () => {
  closeTools(false)
  upsertShow.value = false
})

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
const visibleColumns = computed(() => {
  const names = (page.value?.columns ?? []).map(column => column.name)
  if (typeof route.query.columns !== 'string') return names
  try {
    const requested: unknown = JSON.parse(route.query.columns)
    if (Array.isArray(requested) && requested.length && requested.every(name => typeof name === 'string')) {
      const selected = new Set(requested)
      return names.filter(name => selected.has(name))
    }
  } catch { /* Older or malformed links open with all available columns. */ }
  return names
})
function onVisibleColumns(names: string[]): void {
  const query = { ...route.query }
  if (names.length === page.value?.columns.length) delete query.columns
  else query.columns = JSON.stringify(names)
  void router.push({ query })
}
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
  if (!await fetchDef() || view.value !== 'data') return
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
  if (!pkColumns.value.length || !keys.length || keys.some(key => pkColumns.value.some(name => key[name] === null || key[name] === undefined))) return
  const captured = keys.map(key => Object.fromEntries(pkColumns.value.map(name => [name, key[name]])))
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

// A catalog shortcut opens the same editor as the table toolbar. Consume the
// action once so reloading or returning to the table just shows its rows.
watch(() => [route.query.action, rowsPath.value, catalog.schema] as const, async ([action]) => {
  if (action !== 'insert' || isNewTable.value || view.value !== 'data') return
  const target = catalog.path(rowsPath.value)
  const query = { ...route.query }
  delete query.action
  const failure = await router.replace({ query })
  if (!failure && catalog.path(rowsPath.value) === target && view.value === 'data') await onAddRow()
}, { immediate: true, flush: 'post' })

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
  if (loading.value || !visibleColumns.value.length) return
  try {
    await downloadExport(db.value, {
      format: 'csv', tables: [table.value],
      query: {
        columns: visibleColumns.value,
        filters: filters.value.map(f => ({column:f.column,operator:f.op,value:f.value})),
        sorts: sort.value ? [sort.value] : [],
      },
    }, catalog.schemaAware ? catalog.schema : null)
  } catch (err) { notifyError(err) }
}

function onRowSaved(continueInserting = false): void {
  if (!continueInserting) rowModal.value = { ...rowModal.value, show: false }
  void refetch()
}
</script>

<style scoped>
.table-view { min-width: 0; }
.table-view__head { align-items: center; flex-wrap: wrap; }
.table-view__identity { min-width: 0; }
.table-view__identity h1, .table-view__identity p { overflow-wrap: anywhere; }
.table-view__identity p { margin: 3px 0 0; color: var(--db-muted); }
.table-view__actions { display:flex; align-items:center; flex-wrap:wrap; gap:8px; }
.table-tools { position: relative; }
.table-tools summary { display:flex; align-items:center; justify-content:center; gap:7px; cursor:pointer; min-height:34px; box-sizing:border-box; padding:4px 12px; color:var(--db-ink); background:var(--db-panel); border:1px solid var(--db-line); border-radius:7px; font-size:13px; list-style:none; }
.table-tools summary::-webkit-details-marker { display:none; }
.table-tools[open] summary, .table-tools summary:hover { background:var(--db-tint); color:var(--db-accent); }
.table-tools__list { position:absolute; inset-inline-end:0; top:calc(100% + 5px); z-index:6; display:grid; padding:5px; min-width:190px; border:1px solid var(--db-line); border-radius:8px; background:var(--db-panel); box-shadow:0 8px 24px #121a3426; }
.table-tools__list button { display:flex; align-items:center; text-align:start; gap:8px; font:inherit; font-size:13px; color:var(--db-ink); background:none; border:0; padding:8px; border-radius:5px; cursor:pointer; }
.table-tools__list button:hover, .table-tools__list button:focus-visible { background:var(--db-tint); color:var(--db-accent); }
.table-view__tabs { display:flex; flex-wrap:wrap; gap:4px 12px; min-width:0; border-bottom:1px solid var(--db-line); }
.table-section { display:inline-flex; align-items:center; gap:7px; padding:8px 4px; color:var(--db-muted); background:none; border:0; border-bottom:2px solid transparent; font:inherit; font-size:13px; cursor:pointer; }
.table-section[aria-current=page] { color:var(--db-accent); border-bottom-color:currentColor; font-weight:600; }
.table-view__placeholder { padding:28px; text-align:center; border:1px dashed var(--db-line); border-radius:10px; color:var(--db-muted); }
@media (max-width:600px) {
  .table-view__tabs svg { display:none; }
}
</style>
