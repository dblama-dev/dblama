<template>
  <div class="datagrid">
    <n-alert v-if="!hasPk" type="warning" :show-icon="false" class="datagrid__no-pk">
      {{ t('grid.noPk') }}
    </n-alert>

    <div class="datagrid__toolbar">
      <n-button size="small" type="primary" @click="emit('add-row')">
        {{ t('grid.addRow') }}
      </n-button>
      <n-popconfirm v-if="hasPk && checkedKeys.length > 0" @positive-click="onDeleteSelected">
        <template #trigger>
          <n-button size="small" type="error">
            {{ t('grid.deleteSelected') }} ({{ checkedKeys.length }})
          </n-button>
        </template>
        {{ t('grid.confirmDelete') }}
      </n-popconfirm>
      <n-button size="small" quaternary @click="emit('refresh')">
        {{ t('grid.refresh') }}
      </n-button>
    </div>

    <div class="datagrid__filters">
      <span class="datagrid__filters-label">{{ t('grid.filters') }}</span>
      <div v-for="(draft, i) in drafts" :key="i" class="datagrid__filter">
        <n-select
          v-model:value="draft.column"
          size="small"
          clearable
          :placeholder="t('grid.filters')"
          :options="columnOptions"
          class="datagrid__filter-column"
        />
        <n-select
          v-model:value="draft.op"
          size="small"
          :options="operatorOptions"
          class="datagrid__filter-op"
        />
        <n-input
          v-model:value="draft.value"
          size="small"
          :disabled="isNullOperator(draft.op)"
          class="datagrid__filter-value"
        />
        <n-button size="tiny" quaternary @click="removeDraft(i)">×</n-button>
      </div>
      <n-button size="small" dashed @click="addDraft">+</n-button>
      <n-button size="small" @click="applyFilters">{{ t('grid.apply') }}</n-button>
      <n-button size="small" quaternary @click="resetFilters">{{ t('grid.reset') }}</n-button>
    </div>

    <n-data-table
      remote
      size="small"
      :columns="gridColumns"
      :data="tableData"
      :loading="loading"
      :row-key="rowKey"
      :pagination="pagination"
      :checked-row-keys="checkedKeys"
      class="datagrid__table"
      @update:checked-row-keys="onCheckedRowKeys"
      @update:page="onPage"
      @update:page-size="onPageSize"
      @update:sorter="onSorter"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { h } from 'vue'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import {
  NAlert,
  NButton,
  NDataTable,
  NInput,
  NPopconfirm,
  NSelect,
} from 'naive-ui'

import type { ColumnMeta } from '../api/types'
import { isNullOperator, ROW_FILTER_OPERATORS, type RowsFilter, type RowsSort } from '../lib/tableQuery'

// Grid rows are records keyed by column name plus service fields: the
// row key and the parsed pk map used for delete calls.
interface GridRow extends Record<string, unknown> {
  __key: string
  __pk: Record<string, unknown> | null
}

interface FilterDraft {
  column: string | null
  op: string
  value: string
}

const props = withDefaults(
  defineProps<{
    columns: ColumnMeta[]
    rows: unknown[][]
    totalRows: number
    loading: boolean
    pageIndex: number
    pageSize: number
    pkColumns: string[]
    sort: RowsSort | null
  }>(),
  { sort: null },
)

const emit = defineEmits<{
  'update:page': [page: number]
  'update:page-size': [size: number]
  'update:sort': [sort: RowsSort | null]
  'apply-filters': [filters: RowsFilter[]]
  refresh: []
  'add-row': []
  'edit-row': [row: GridRow]
  'delete-row': [pk: Record<string, unknown>]
  'delete-rows': [pks: Record<string, unknown>[]]
}>()

const { t } = useI18n()

const hasPk = computed(() => props.pkColumns.length > 0)

// --- Grid rows ---------------------------------------------------------

const tableData = computed<GridRow[]>(() => {
  const offset = (props.pageIndex - 1) * props.pageSize
  return props.rows.map((row, i) => {
    const record: GridRow = { __key: '', __pk: null }
    props.columns.forEach((meta, j) => {
      record[meta.name] = row[j]
    })
    if (hasPk.value) {
      const pk: Record<string, unknown> = {}
      for (const name of props.pkColumns) {
        const idx = props.columns.findIndex((meta) => meta.name === name)
        pk[name] = idx >= 0 ? row[idx] : null
      }
      record.__pk = pk
      record.__key = props.pkColumns.map((name) => String(pk[name])).join(' | ')
    } else {
      record.__key = `__idx_${offset + i}`
    }
    return record
  })
})

function rowKey(row: GridRow): string {
  return row.__key
}

// --- Columns -----------------------------------------------------------

// Bare hex literals (e.g. 0x098F6BCD4621D373CADE4E832627B4F6) render in
// monospace, truncated by CSS, with the full value on hover.
const HEX_RE = /^0x[0-9a-fA-F]+$/

function renderCell(value: unknown): ReturnType<typeof h> {
  if (value === null || value === undefined) {
    return h('span', { class: 'datagrid__null', style: 'font-style: italic' }, t('grid.null'))
  }
  if (typeof value === 'string' && HEX_RE.test(value)) {
    return h('code', { class: 'datagrid__hex', title: value }, value)
  }
  return String(value)
}

const gridColumns = computed<DataTableColumns<GridRow>>(() => {
  const cols: DataTableColumns<GridRow> = []
  if (hasPk.value) {
    cols.push({ type: 'selection' })
  }
  for (const meta of props.columns) {
    cols.push({
      title: meta.name,
      key: meta.name,
      sorter: 'default',
      sortOrder:
        props.sort?.column === meta.name ? (props.sort.desc ? 'descend' : 'ascend') : false,
      render: (row) => renderCell(row[meta.name]),
    })
  }
  if (hasPk.value) {
    cols.push({
      title: t('grid.actions'),
      key: '__actions',
      render: (row) =>
        h('span', { class: 'datagrid__row-actions' }, [
          h(
            NButton,
            { size: 'tiny', quaternary: true, onClick: () => emit('edit-row', row) },
            { default: () => t('grid.edit') },
          ),
          h(
            NButton,
            {
              size: 'tiny',
              quaternary: true,
              type: 'error',
              onClick: () => {
                if (row.__pk) emit('delete-row', row.__pk)
              },
            },
            { default: () => t('grid.delete') },
          ),
        ]),
    })
  }
  return cols
})

// --- Selection ---------------------------------------------------------

const checkedKeys = ref<string[]>([])

function onCheckedRowKeys(keys: (string | number)[]): void {
  checkedKeys.value = keys.map(String)
}

// A refetch replaces the rows: drop checked keys that no longer exist so
// the selection can never reference deleted rows.
watch(
  () => props.rows,
  () => {
    const valid = new Set(tableData.value.map((row) => row.__key))
    checkedKeys.value = checkedKeys.value.filter((key) => valid.has(key))
  },
)

function onDeleteSelected(): void {
  const byKey = new Map(tableData.value.map((row) => [row.__key, row]))
  const pks = checkedKeys.value
    .map((key) => byKey.get(key)?.__pk)
    .filter((pk): pk is Record<string, unknown> => pk !== null && pk !== undefined)
  if (pks.length > 0) {
    emit('delete-rows', pks)
  }
  checkedKeys.value = []
}

// --- Pagination and sorting --------------------------------------------

const pagination = computed(() => ({
  page: props.pageIndex,
  pageSize: props.pageSize,
  itemCount: props.totalRows,
  pageSizes: [25, 50, 100, 200],
  showSizePicker: true,
}))

function onPage(page: number): void {
  emit('update:page', page)
}

function onPageSize(size: number): void {
  emit('update:page-size', size)
}

interface SorterEvent {
  column?: { key?: string | number }
  order?: 'ascend' | 'descend' | false
}

function onSorter(event: SorterEvent): void {
  const key = event.column?.key
  if (key === undefined || !event.order) {
    emit('update:sort', null)
    return
  }
  emit('update:sort', { column: String(key), desc: event.order === 'descend' })
}

// --- Filter row --------------------------------------------------------

const drafts = ref<FilterDraft[]>([emptyDraft()])

function emptyDraft(): FilterDraft {
  return { column: null, op: '=', value: '' }
}

const columnOptions = computed<SelectOption[]>(() =>
  props.columns.map((meta) => ({ label: meta.name, value: meta.name })),
)

const operatorOptions = computed<SelectOption[]>(() =>
  ROW_FILTER_OPERATORS.map((op) => ({ label: op, value: op })),
)

function addDraft(): void {
  drafts.value.push(emptyDraft())
}

function removeDraft(index: number): void {
  drafts.value.splice(index, 1)
  if (drafts.value.length === 0) {
    drafts.value.push(emptyDraft())
  }
}

function applyFilters(): void {
  const filters: RowsFilter[] = drafts.value
    .filter((draft) => draft.column !== null && draft.column !== '')
    .map((draft) => ({
      column: draft.column as string,
      op: draft.op,
      value: isNullOperator(draft.op) ? '' : draft.value,
    }))
  emit('apply-filters', filters)
}

function resetFilters(): void {
  drafts.value = [emptyDraft()]
  emit('apply-filters', [])
}
</script>

<style scoped>
.datagrid {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.datagrid__toolbar,
.datagrid__filters {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.datagrid__filters-label {
  opacity: 0.7;
}

.datagrid__filter {
  display: flex;
  align-items: center;
  gap: 4px;
}

.datagrid__filter-column {
  min-width: 140px;
}

.datagrid__filter-op {
  min-width: 120px;
}

.datagrid__filter-value {
  min-width: 160px;
}

.datagrid__null {
  opacity: 0.6;
}

.datagrid__hex {
  display: inline-block;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  vertical-align: bottom;
}

.datagrid__row-actions {
  white-space: nowrap;
}
</style>
