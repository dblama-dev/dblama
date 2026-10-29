<template>
  <div class="datagrid">
    <n-alert v-if="!hasPk" type="warning" :show-icon="false" class="datagrid__no-pk">
      {{ t('grid.noPk') }}
    </n-alert>

    <div class="datagrid__toolbar">
      <n-button size="small" type="primary" @click="emit('add-row')">
        {{ t('grid.addRow') }}
      </n-button>
      <n-popconfirm v-if="checkedKeys.length > 0" @positive-click="onDeleteSelected">
        <template #trigger>
          <n-button size="small" type="error">
            {{ t('grid.deleteSelected') }} ({{ checkedKeys.length }})
          </n-button>
        </template>
        {{ t('grid.confirmDelete') }} {{ tableName }} ({{ checkedKeys.length }})
      </n-popconfirm>
      <n-popconfirm v-if="checkedKeys.length > 0" @positive-click="copySelected">
        <template #trigger><n-button size="small">{{ t('grid.copySelected') }} ({{ checkedKeys.length }})</n-button></template>
        {{ t('grid.copyConfirm') }}
      </n-popconfirm>
      <n-button v-if="checkedKeys.length" size="small" @click="editSelected">{{ t('navigationWorkflows.editSelected') }} ({{ checkedKeys.length }})</n-button>
      <n-button size="small" quaternary @click="emit('refresh')">
        {{ t('grid.refresh') }}
      </n-button>
      <n-button size="small" data-testid="grid-export" @click="emit('export-selection')">{{ t('grid.exportSelection') }}</n-button>
    </div>

    <div class="datagrid__filters">
      <span class="datagrid__filters-label">{{ t('grid.filters') }}</span>
      <div v-for="(draft, i) in drafts" :key="i" class="datagrid__filter">
        <n-select
          :aria-label="t('navigationWorkflows.filterColumn')"
          v-model:value="draft.column"
          size="small"
          clearable
          :placeholder="t('grid.filters')"
          :options="columnOptions"
          class="datagrid__filter-column"
        />
        <n-select
          :aria-label="t('navigationWorkflows.filterOperator')"
          v-model:value="draft.op"
          size="small"
          :options="operatorOptions"
          class="datagrid__filter-op"
        />
        <n-input
          :aria-label="t('navigationWorkflows.filterValue')"
          @keydown.enter="applyFilters"
          v-model:value="draft.value"
          size="small"
          :disabled="isNullOperator(draft.op)"
          class="datagrid__filter-value"
        />
        <n-button :aria-label="t('navigationWorkflows.removeFilter')" size="tiny" quaternary @click="removeDraft(i)">×</n-button>
      </div>
      <n-button :aria-label="t('navigationWorkflows.addFilter')" size="small" dashed @click="addDraft">+</n-button>
      <n-button size="small" @click="applyFilters">{{ t('grid.apply') }}</n-button>
      <n-button size="small" quaternary @click="resetFilters">{{ t('grid.reset') }}</n-button>
    </div>

    <div class="datagrid__pagination"><span>{{ t('grid.total') }}: {{ totalRows }}</span><n-pagination v-bind="pagination" @update:page="onPage" @update:page-size="onPageSize" /></div>
    <n-data-table
      remote
      size="small"
      :columns="gridColumns"
      :data="tableData"
      :loading="loading"
      :row-key="rowKey"
      :pagination="pagination"
      :scroll-x="scrollWidth"
      :checked-row-keys="checkedKeys"
      class="datagrid__table"
      @update:checked-row-keys="onCheckedRowKeys"
      @update:page="onPage"
      @update:page-size="onPageSize"
      @update:sorter="onSorter"
    />
    <div class="datagrid__total">{{ t('grid.total') }}: {{ totalRows }}</div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { h } from 'vue'
import type { DataTableColumns, DataTableSortState, SelectOption } from 'naive-ui'
import {
  NAlert,
  NButton,
  NDataTable,
  NInput,
  NPopconfirm,
  NPagination,
  NSelect,
} from 'naive-ui'

import { useMetadataStore } from '../stores/metadata'
import BlobValue from './BlobValue.vue'
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
    tableName?: string
    filters?: RowsFilter[]
    sort: RowsSort | null
  }>(),
  { sort: null, filters: () => [], tableName: '' },
)

const emit = defineEmits<{
  'update:page': [page: number]
  'update:page-size': [size: number]
  'update:sort': [sort: RowsSort | null]
  'apply-filters': [filters: RowsFilter[]]
  refresh: []
  'add-row': []
  'edit-row': [row: GridRow]
  'edit-rows': [keys: Record<string,unknown>[]]
  'copy-row': [row: GridRow]
  'copy-rows': [keys: Record<string,unknown>[]]
  'export-selection': []
  'delete-row': [pk: Record<string, unknown>]
  'delete-rows': [pks: Record<string, unknown>[]]
}>()

const { t } = useI18n()
const metadata = useMetadataStore()
void metadata.load()

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
      record.__key = props.pkColumns.length === 1 ? String(pk[props.pkColumns[0]!]) : JSON.stringify(props.pkColumns.map(name => pk[name]))
    } else {
      record.__key = `__idx_${offset + i}`
      record.__pk = Object.fromEntries(props.columns.map((meta, j) => [meta.name, row[j]]))
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

function renderCell(value: unknown): ReturnType<typeof h> | string {
  if (value === null || value === undefined) {
    return h('span', { class: 'datagrid__null', style: 'font-style: italic' }, t('grid.null'))
  }
  if (typeof value === 'string' && HEX_RE.test(value)) {
    return h(BlobValue, { hex: value })
  }
  const text = typeof value === 'object' ? JSON.stringify(value) : String(value)
  return h('details', {class:'datagrid__cell'}, [h('summary',{title:text},text),h('pre', {tabindex:0},text)])
}

const gridColumns = computed<DataTableColumns<GridRow>>(() => {
  const cols: DataTableColumns<GridRow> = []
  cols.push({ type: 'selection' })
  for (const meta of props.columns) {
    cols.push({
      width: Math.max(140,Math.min(300,meta.name.length*9+70)),
      title: meta.name,
      key: meta.name,
      sorter: 'default',
      sortOrder:
        props.sort?.column === meta.name ? (props.sort.desc ? 'descend' : 'ascend') : false,
      render: (row) => renderCell(row[meta.name]),
    })
  }
  {
    cols.push({
      title: t('grid.actions'),
      key: '__actions',
      width: 240,
      fixed: 'right',
      render: (row) =>
        h('span', { class: 'datagrid__row-actions' }, [
          h(NButton, { size: 'tiny', quaternary: true, 'data-testid': 'copy-row', onClick: () => emit('copy-row', row) }, { default: () => t('grid.copy') }),
          h(
            NButton,
            { size: 'tiny', quaternary: true, onClick: () => emit('edit-row', row) },
            { default: () => t('grid.edit') },
          ),
          h(NPopconfirm, {onPositiveClick: () => { if (row.__pk) emit('delete-row',row.__pk) }}, {
            trigger: () => h(NButton,{size:'tiny',quaternary:true,type:'error'},{default:()=>t('grid.delete')}),
            default: () => `${t('grid.confirmDelete')} ${props.tableName} ${JSON.stringify(row.__pk)}`,
          }),
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

// A new result set clears selection. In keyless tables positional row keys
// can otherwise silently select a different row after a sort or filter.
watch(
  () => props.rows,
  () => {
    checkedKeys.value = []
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

function copySelected(): void {
  const selected = new Set(checkedKeys.value)
  const keys = tableData.value.filter(row => selected.has(row.__key)).map(row => row.__pk).filter((key): key is Record<string,unknown> => key !== null)
  if (keys.length) emit('copy-rows', keys)
  checkedKeys.value=[]
}

function editSelected(): void {
  const selected=new Set(checkedKeys.value)
  const keys=tableData.value.filter(row=>selected.has(row.__key)).map(row=>row.__pk).filter((key): key is Record<string,unknown>=>key!==null)
  if(keys.length) emit('edit-rows',keys.map(key=>({...key})))
}
const scrollWidth = computed(() => 280 + props.columns.reduce((sum,meta)=>sum+Math.max(140,Math.min(300,meta.name.length*9+70)),0))

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

function onSorter(event: DataTableSortState | DataTableSortState[] | null): void {
  // This grid supports one server-side sort. Naive emits null when cleared.
  const current = Array.isArray(event) ? event[0] : event
  if (!current?.order) {
    emit('update:sort', null)
    return
  }
  emit('update:sort', { column: String(current.columnKey), desc: current.order === 'descend' })
}

// --- Filter row --------------------------------------------------------

const drafts = ref<FilterDraft[]>([emptyDraft()])
watch(() => props.filters, value => { drafts.value = value.length ? value.map(filter=>({...filter})) : [emptyDraft()] }, {immediate:true,deep:true})

function emptyDraft(): FilterDraft {
  return { column: null, op: '=', value: '' }
}

const columnOptions = computed<SelectOption[]>(() =>
  props.columns.map((meta) => ({ label: meta.name, value: meta.name })),
)

const operatorOptions = computed<SelectOption[]>(() =>
  metadata.data?.operators.map(op => ({ label:op.label,value:op.value })) ?? ROW_FILTER_OPERATORS.map(op => ({label:op,value:op})),
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
.datagrid__pagination {display:flex;justify-content:space-between;gap:8px;flex-wrap:wrap;}
.datagrid__toolbar {position:sticky;top:0;z-index:2;background:var(--n-color,#fff);padding:6px 0;}
.datagrid__table {max-width:100%;overflow:auto;}
.datagrid__cell summary {overflow:hidden;text-overflow:ellipsis;white-space:nowrap;cursor:pointer;max-width:260px;}
.datagrid__cell pre {white-space:pre-wrap;overflow:auto;max-height:240px;max-width:300px;word-break:break-word;}
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
