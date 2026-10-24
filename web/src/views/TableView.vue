<template>
  <section class="table-view">
    <n-breadcrumb>
      <n-breadcrumb-item>{{ db }}</n-breadcrumb-item>
      <n-breadcrumb-item>{{ table }}</n-breadcrumb-item>
    </n-breadcrumb>
    <n-tabs type="line" :value="view" @update:value="switchView">
      <n-tab name="data">{{ t('table.data') }}</n-tab>
      <n-tab name="structure">{{ t('table.structure') }}</n-tab>
    </n-tabs>
    <DataGrid
      v-if="view === 'data'"
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
    <p v-else class="table-view__stub">
      {{ view }}: {{ db }}.{{ table }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useNotification } from 'naive-ui'
import { NBreadcrumb, NBreadcrumbItem, NTab, NTabs } from 'naive-ui'

import { api, ApiError } from '../api/client'
import type { Page } from '../api/types'
import DataGrid from '../components/DataGrid.vue'
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

function switchView(next: string | number): void {
  void router.push({ query: { ...route.query, view: next } })
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

async function loadPk(): Promise<void> {
  try {
    const pk = await api<{ columns: string[] }>('GET', pkPath.value)
    pkColumns.value = pk.columns
  } catch (err) {
    notifyError(err)
  }
}

async function refetch(): Promise<void> {
  loading.value = true
  try {
    page.value = await api<Page>('GET', `${rowsPath.value}/rows?${buildRowsQuery(queryState())}`)
  } catch (err) {
    notifyError(err)
  } finally {
    loading.value = false
  }
}

async function reload(): Promise<void> {
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

// RowModal lands in Task 13; the grid already reports the intents.
function onAddRow(): void {}

function onEditRow(row: unknown): void {
  void row
}
</script>

<style scoped>
.table-view {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.table-view__stub {
  opacity: 0.6;
}
</style>
