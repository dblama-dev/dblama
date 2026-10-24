<template>
  <section class="table-view">
    <n-breadcrumb>
      <n-breadcrumb-item>{{ db }}</n-breadcrumb-item>
      <n-breadcrumb-item>{{ isNewTable ? t('table.new') : table }}</n-breadcrumb-item>
    </n-breadcrumb>
    <n-tabs type="line" :value="view" @update:value="switchView">
      <n-tab name="data">{{ t('table.data') }}</n-tab>
      <n-tab name="structure">{{ t('table.structure') }}</n-tab>
    </n-tabs>
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
      @update:page="onPage"
      @update:page-size="onPageSize"
      @update:sort="onSort"
      @apply-filters="onFilters"
      @refresh="refetch"
      @add-row="onAddRow"
      @edit-row="onEditRow"
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
    <RowModal
      v-if="!isNewTable"
      :show="rowModal.show"
      :mode="rowModal.mode"
      :rows-path="rowsPath"
      :columns="modalColumns"
      :row="rowModal.row"
      :row-pk="rowModal.pk"
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
import { NBreadcrumb, NBreadcrumbItem, NTab, NTabs } from 'naive-ui'

import { api, ApiError } from '../api/client'
import type { ColumnDef, Page, TableDef } from '../api/types'
import { useCatalogStore } from '../stores/catalog'
import DataGrid from '../components/DataGrid.vue'
import RowModal from '../components/RowModal.vue'
import StructureEditor from './structure/StructureEditor.vue'
import { buildRowsQuery, type RowsFilter, type RowsSort } from '../lib/tableQuery'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const notification = useNotification()

// vue-router hands the path params over already decoded; re-encode them
// for the api path (identifiers may contain %, spaces, ...).
const db = computed(() => String(route.params.db ?? ''))
const table = computed(() => String(route.params.t ?? ''))
const view = computed(() => (route.query.view === 'structure' ? 'structure' : 'data'))
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
const pkPath = computed(() => `${rowsPath.value}/pk`)

// All fetch logic for the data view lives here: DataGrid is presentational
// and reports query-state changes back as events.
const pkColumns = ref<string[]>([])
const page = ref<Page | null>(null)
const loading = ref(false)
const pageIndex = ref(1)
const pageSize = ref(50)
const sort = ref<RowsSort | null>(null)
const filters = ref<RowsFilter[]>([])

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

// Stale-response guard: every fetch captures the counter value taken at its
// start; a response that arrives after a newer request has started (seq has
// moved on) is discarded — both its data and its error. The counter is
// shared by the pk and rows fetches, so a slow pk response can never
// overwrite a fresher one either. `loading` also follows the latest request.
let fetchSeq = 0

async function loadPk(): Promise<void> {
  const seq = ++fetchSeq
  try {
    const pk = await api<{ columns: string[] }>('GET', pkPath.value)
    if (seq !== fetchSeq) return
    pkColumns.value = pk.columns
  } catch (err) {
    if (seq !== fetchSeq) return
    notifyError(err)
  }
}

async function refetch(): Promise<void> {
  const seq = ++fetchSeq
  loading.value = true
  try {
    const next = await api<Page>('GET', `${rowsPath.value}/rows?${buildRowsQuery(queryState())}`)
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
  // The _new placeholder has no table behind it — no pk or rows fetch.
  if (isNewTable.value) return
  pageIndex.value = 1
  sort.value = null
  filters.value = []
  await loadPk()
  await refetch()
}

onMounted(() => {
  void reload()
})

watch([db, table], () => {
  void reload()
})

async function onPage(next: number): Promise<void> {
  pageIndex.value = next
  await refetch()
}

async function onPageSize(size: number): Promise<void> {
  pageSize.value = size
  pageIndex.value = 1
  await refetch()
}

async function onSort(next: RowsSort | null): Promise<void> {
  sort.value = next
  pageIndex.value = 1
  await refetch()
}

async function onFilters(next: RowsFilter[]): Promise<void> {
  filters.value = next
  pageIndex.value = 1
  await refetch()
}

async function deleteRows(pks: Record<string, unknown>[]): Promise<void> {
  if (pks.length === 0) return
  try {
    await api('DELETE', `${rowsPath.value}/rows`, { where: pks })
  } catch (err) {
    notifyError(err)
    return
  }
  await refetch()
  // A delete can empty the current page: step back one page if so.
  if ((page.value?.rows.length ?? 0) === 0 && pageIndex.value > 1) {
    pageIndex.value -= 1
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
const modalColumns = computed<ColumnDef[]>(() => defColumns.value ?? page.value?.columns ?? [])

async function fetchDef(): Promise<void> {
  try {
    const def = await api<TableDef>('GET', rowsPath.value)
    defColumns.value = def.columns
  } catch {
    // Definition unavailable: ColumnMeta-only rendering is good enough.
    defColumns.value = null
  }
}

function onAddRow(): void {
  defColumns.value = null
  void fetchDef()
  rowModal.value = { show: true, mode: 'insert', row: null, pk: null }
}

function onEditRow(row: EditableRow): void {
  const values: Record<string, unknown> = {}
  for (const meta of page.value?.columns ?? []) {
    values[meta.name] = row[meta.name]
  }
  defColumns.value = null
  void fetchDef()
  rowModal.value = { show: true, mode: 'edit', row: values, pk: row.__pk }
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

.table-view__placeholder {
  padding: 24px;
  opacity: 0.6;
}
</style>
