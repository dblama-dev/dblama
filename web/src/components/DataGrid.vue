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
      <n-button v-if="checkedKeys.length" size="small" :disabled="!canBulkEdit" @click="editSelected">{{ t('navigationWorkflows.editSelected') }} ({{ checkedKeys.length }})</n-button>
      <small v-if="checkedKeys.length && !canBulkEdit" data-bulk-edit-unavailable>{{ t('navigationWorkflows.bulkNeedsPk') }}</small>
      <n-button size="small" quaternary @click="emit('refresh')">
        {{ t('grid.refresh') }}
      </n-button>
      <n-button
        ref="columnTrigger" size="small" data-testid="grid-columns"
        :aria-expanded="choosingColumns" :aria-controls="columnChooserID"
        :disabled="!columns.length" @click="toggleColumnChooser"
      >
        {{ t('structure.columns') }} <span v-if="visibleMetadata.length !== columns.length" class="datagrid__column-count">{{ visibleMetadata.length }} / {{ columns.length }}</span>
      </n-button>
      <n-button size="small" data-testid="grid-export" :disabled="loading || !visibleMetadata.length" @click="emit('export-selection')">{{ t('rowWorkflows.exportVisible') }}</n-button>
    </div>

    <form
      v-if="choosingColumns" :id="columnChooserID" ref="columnChooser" class="datagrid__chooser"
      :aria-label="t('rowWorkflows.visibleColumns')" data-testid="grid-column-chooser"
      @submit.prevent="applyColumnSelection" @keydown.esc.prevent="closeColumnChooser"
    >
      <div class="datagrid__chooser-heading">
        <strong>{{ t('rowWorkflows.visibleColumns') }}</strong>
        <span>{{ t('rowWorkflows.chosenCount', { chosen: columnDraft.length, total: columns.length }) }}</span>
      </div>
      <n-input v-if="columns.length > 8" v-model:value="columnSearch" size="small" clearable :input-props="{ 'aria-label': t('rowWorkflows.findColumn') }" :placeholder="t('rowWorkflows.findColumn')" />
      <div class="datagrid__chooser-list">
        <label v-for="column in matchingColumns" :key="column.name">
          <input v-model="columnDraft" type="checkbox" :value="column.name">
          {{ column.name }}
        </label>
        <span v-if="!matchingColumns.length">{{ t('rowWorkflows.noMatchingColumns') }}</span>
      </div>
      <div class="datagrid__chooser-footer">
        <div class="datagrid__chooser-shortcuts">
          <n-button size="small" quaternary data-testid="show-all-columns" :disabled="columnDraft.length === columns.length" @click="columnDraft = columns.map(col => col.name)">{{ t('rowWorkflows.showAllColumns') }}</n-button>
          <n-button size="small" quaternary data-testid="clear-columns" :disabled="!columnDraft.length" @click="columnDraft = []">{{ t('rowWorkflows.clearColumns') }}</n-button>
        </div>
        <n-button size="small" @click="closeColumnChooser">{{ t('row.cancel') }}</n-button>
        <n-button attr-type="submit" size="small" type="primary" data-testid="apply-columns" :disabled="!columnDraft.length">{{ t('grid.apply') }}</n-button>
      </div>
    </form>
    <n-alert v-if="columns.length && !visibleMetadata.length" type="warning" :show-icon="false">{{ t('rowWorkflows.columnsMissing') }}</n-alert>

    <button class="datagrid__filter-toggle" type="button" :aria-expanded="mobileFiltersOpen" :aria-controls="filterID" @click="mobileFiltersOpen = !mobileFiltersOpen">
      {{ t('grid.filters') }}<span v-if="filters.length"> ({{ filters.length }})</span>
    </button>
    <div :id="filterID" class="datagrid__filters" :class="{ 'datagrid__filters--open': mobileFiltersOpen }">
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

    <details v-if="statement" class="datagrid__statement">
      <summary>{{ t('grid.browseSQL') }}</summary>
      <pre tabindex="0">{{ statement.sql }}</pre>
      <span>{{ t('grid.parameters') }}</span> <output>{{ JSON.stringify(statement.parameters) }}</output>
    </details>
    <div class="datagrid__pagination"><span>{{ t('grid.total') }}: {{ totalRows }}</span><n-pagination v-bind="pagination" @update:page="onPage" @update:page-size="onPageSize" /></div>
    <n-data-table v-if="!columns.length || visibleMetadata.length"
      remote
      size="small"
      :columns="gridColumns"
      :data="tableData"
      :loading="loading"
      :row-key="rowKey"
      :pagination="pagination"
      :scroll-x="scrollWidth"
      max-height="max(240px, calc(100vh - 360px))"
      :checked-row-keys="checkedKeys"
      class="datagrid__table"
      @update:checked-row-keys="onCheckedRowKeys"
      @update:page="onPage"
      @update:page-size="onPageSize"
      @update:sorter="onSorter"
    />
    <div class="datagrid__total">{{ t('grid.total') }}: {{ totalRows }}</div>
    <n-modal v-model:show="inspectorShow" preset="card" :title="`${t('grid.preview')}: ${inspectedColumn}`" style="width: min(800px, 96vw)">
      <n-select v-if="inspectedRow" data-testid="grid-inspect-column" :value="inspectedColumn" :aria-label="t('structure.columns')" :options="columnOptions" @update:value="selectInspectedColumn" />
      <pre class="datagrid__full-value" data-testid="grid-inspected-value" tabindex="0" style="white-space: pre-wrap; overflow: auto; max-height: 70vh; overflow-wrap: anywhere">{{ inspectedValue }}</pre>
      <small v-if="inspectedRow?.[inspectedColumn] === ''">{{ t('rowWorkflows.emptyString') }}</small>
      <div v-if="inspectedFilter" class="datagrid__inspector-action">
        <n-button size="small" data-testid="filter-inspected-value" :disabled="filtersEdited || alreadyFiltered || loading" @click="filterByInspectedValue">
          {{ t(alreadyFiltered ? 'rowWorkflows.alreadyFiltered' : 'rowWorkflows.filterByValue') }}
        </n-button>
        <small v-if="filtersEdited">{{ t('rowWorkflows.unappliedFilters') }}</small>
      </div>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { h } from 'vue'
import type { DataTableColumns, DataTableSortState, SelectOption } from 'naive-ui'
import {
  NAlert,
  NButton,
  NDataTable,
  NInput,
  NModal,
  NPopconfirm,
  NPagination,
  NSelect,
} from 'naive-ui'

import { useMetadataStore } from '../stores/metadata'
import BlobValue from './BlobValue.vue'
import GridRowActions from './GridRowActions.vue'
import type { ColumnMeta, Page } from '../api/types'
import { filterForCell, isNullOperator, ROW_FILTER_OPERATORS, type RowsFilter, type RowsSort } from '../lib/tableQuery'

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
    statement?: Page['statement']
    rows: unknown[][]
    totalRows: number
    loading: boolean
    pageIndex: number
    pageSize: number
    pkColumns: string[]
    visibleColumns?: string[]
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
  'update:visible-columns': [names: string[]]
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

const visibleMetadata = computed(() => {
  if (!props.visibleColumns) return props.columns
  const selected = new Set(props.visibleColumns)
  return props.columns.filter(column => selected.has(column.name))
})
const choosingColumns = ref(false)
const columnChooserID = useId()
const columnTrigger = ref<InstanceType<typeof NButton> | null>(null)
const columnChooser = ref<HTMLFormElement | null>(null)
const columnDraft = ref<string[]>([])
const columnSearch = ref('')
const matchingColumns = computed(() => {
  const needle = columnSearch.value.trim().toLowerCase()
  return props.columns.filter(column => column.name.toLowerCase().includes(needle))
})
async function toggleColumnChooser(): Promise<void> {
  choosingColumns.value = !choosingColumns.value
  if (!choosingColumns.value) return
  columnDraft.value = visibleMetadata.value.map(column => column.name)
  columnSearch.value = ''
  await nextTick()
  columnChooser.value?.querySelector('input')?.focus()
}
function closeColumnChooser(): void {
  choosingColumns.value = false
  columnTrigger.value?.$el.focus()
}
function applyColumnSelection(): void {
  const selected = new Set(columnDraft.value)
  const names = props.columns.map(column => column.name).filter(name => selected.has(name))
  if (!names.length) return
  emit('update:visible-columns', names)
  closeColumnChooser()
}
watch([() => props.tableName, () => props.columns, () => props.visibleColumns], () => {
  choosingColumns.value = false
})

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

const inspectedRow = ref<GridRow | null>(null)
const inspectedValue = ref('')
const inspectedColumn = ref('')
const inspectorShow = ref(false)
const CELL_PREVIEW_LENGTH = 120

function cellText(value: unknown): string {
  if (value === null || value === undefined) return t('grid.null')
  return typeof value === 'object' ? JSON.stringify(value) : String(value)
}

function needsInspector(text: string): boolean {
  return text.length > CELL_PREVIEW_LENGTH || text.split(/\r?\n/).length > 3
}

function cellPreview(text: string): string {
  if (!needsInspector(text)) return text
  return `${text.replace(/\s+/g, ' ').slice(0, CELL_PREVIEW_LENGTH)}…`
}

function inspectCell(row: GridRow, column: string): void {
  inspectedRow.value = { ...row }
  inspectedColumn.value = column
  inspectedValue.value = cellText(row[column])
  inspectorShow.value = true
}
function selectInspectedColumn(column: string): void {
  if (inspectedRow.value) inspectCell(inspectedRow.value, column)
}

function renderCell(row: GridRow, column: string): ReturnType<typeof h> | string {
  const value = row[column]
  if (value === null || value === undefined) {
    return h('span', { class: 'datagrid__null', style: 'font-style: italic' }, t('grid.null'))
  }
  if (typeof value === 'string' && HEX_RE.test(value)) {
    return h(BlobValue, { hex: value })
  }
  const text = cellText(value)
  if (!needsInspector(text)) {
    return h('span', {
      class: 'datagrid__cell-short',
      style: { display: 'block', maxWidth: '260px', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' },
    }, text)
  }
  // The full text lives in the inspector outside the table. Keeping it out
  // of a hidden cell subtree avoids duplicate text and unnamed group cells.
  return h('span', {
    class: 'datagrid__cell-preview',
    style: { display: 'flex', alignItems: 'center', gap: '6px', maxWidth: '260px', minWidth: '0' },
  }, [
    h('span', {
      style: { flex: '1 1 auto', minWidth: '0', overflow: 'hidden', whiteSpace: 'nowrap', textOverflow: 'ellipsis' },
    }, cellPreview(text)),
    h('button', {
      type: 'button',
      class: 'datagrid__inspect',
      style: { flexShrink: '0' },
      'aria-label': `${t('grid.preview')}: ${column}`,
      onClick: () => inspectCell(row, column),
    }, t('grid.preview')),
  ])
}

const rowActionsWidth = 148
const gridColumns = computed<DataTableColumns<GridRow>>(() => {
  const cols: DataTableColumns<GridRow> = []
  cols.push({ type: 'selection' })
  for (const meta of visibleMetadata.value) {
    cols.push({
      width: Math.max(140,Math.min(300,meta.name.length*9+70)),
      title: meta.name,
      key: meta.name,
      sorter: 'default',
      sortOrder:
        props.sort?.column === meta.name ? (props.sort.desc ? 'descend' : 'ascend') : false,
      cellProps: (row) => ({ 'aria-label': cellPreview(cellText(row[meta.name])) }),
      render: (row) => renderCell(row, meta.name),
    })
  }
  cols.push({
    title: t('grid.actions'),
    key: '__actions',
    width: rowActionsWidth,
    fixed: 'right',
    render: row => h(GridRowActions, {
      tableName: props.tableName ?? '',
      target: row.__pk,
      canPreview: visibleMetadata.value.length > 0,
      onAction: action => {
        if (action === 'edit') emit('edit-row', row)
        else if (action === 'copy') emit('copy-row', row)
        else if (visibleMetadata.value[0]) inspectCell(row, visibleMetadata.value[0].name)
      },
      onDelete: key => emit('delete-row', key),
    }),
  })
  return cols
})

// --- Selection ---------------------------------------------------------

const checkedKeys = ref<string[]>([])
const filterID = useId()
const mobileFiltersOpen = ref(false)

function onCheckedRowKeys(keys: (string | number)[]): void {
  checkedKeys.value = keys.map(String)
}

// A new result set clears selection. In keyless tables positional row keys
// can otherwise silently select a different row after a sort or filter.
watch(
  () => props.rows,
  () => {
    checkedKeys.value = []
    inspectorShow.value = false
    inspectedRow.value = null
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
  if (!canBulkEdit.value) return
  const selected=new Set(checkedKeys.value)
  const keys=tableData.value.filter(row=>selected.has(row.__key)).map(row=>row.__pk).filter((key): key is Record<string,unknown>=>key!==null)
  if(keys.length) emit('edit-rows',keys.map(key=>({...key})))
}
const canBulkEdit = computed(() => hasPk.value && checkedKeys.value.length > 0 && checkedKeys.value.every(key => {
  const row = tableData.value.find(row => row.__key === key)
  return row && props.pkColumns.every(name => row.__pk?.[name] !== null && row.__pk?.[name] !== undefined)
}))
const scrollWidth = computed(() => 48 + rowActionsWidth + visibleMetadata.value.reduce((sum,meta)=>sum+Math.max(140,Math.min(300,meta.name.length*9+70)),0))

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

const inspectedFilter = computed(() => {
  const col = props.columns.find(meta => meta.name === inspectedColumn.value)
  if (!col || !inspectedRow.value) return null
  const filter = filterForCell(col.name, col.type, inspectedRow.value[col.name])
  return filter && operatorOptions.value.some(option => option.value === filter.op) ? filter : null
})
function sameFilter(a: FilterDraft, b: RowsFilter): boolean {
  return a.column === b.column && a.op === b.op && (isNullOperator(a.op) || a.value === b.value)
}
const filtersEdited = computed(() => {
  const entries = drafts.value.filter(draft => draft.column !== null || draft.op !== '=' || draft.value !== '')
  return entries.length !== props.filters.length || entries.some((draft, i) => !sameFilter(draft, props.filters[i]!))
})
const alreadyFiltered = computed(() => {
  const inspected = inspectedFilter.value
  return !!inspected && props.filters.some(filter => sameFilter(filter, inspected))
})
function filterByInspectedValue(): void {
  if (!inspectedFilter.value || alreadyFiltered.value || filtersEdited.value || props.loading) return
  emit('apply-filters', [...props.filters, inspectedFilter.value])
  inspectorShow.value = false
}

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
.datagrid__statement { margin: 6px 0; font-size: 13px; }
.datagrid__statement summary { width: fit-content; cursor: pointer; }
.datagrid__statement pre { padding: 8px 10px; margin: 6px 0; border-left: 2px solid currentColor; background: #8881; white-space: pre-wrap; overflow-wrap: anywhere; }
.datagrid__statement output { font-family: monospace; overflow-wrap: anywhere; }
.datagrid__pagination {display:flex;justify-content:space-between;gap:8px;flex-wrap:wrap;}
.datagrid__toolbar {position:sticky;top:0;z-index:2;background:var(--n-color,#fff);padding:6px 0;}
.datagrid__table {max-width:100%;overflow:auto;}
.datagrid__inspector-action { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
.datagrid__column-count { margin-inline-start: 6px; font-variant-numeric: tabular-nums; }
.datagrid__chooser { display: grid; gap: 10px; padding: 12px; border: 1px solid #8886; border-radius: 4px; }
.datagrid__chooser-heading, .datagrid__chooser-footer { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.datagrid__chooser-heading { justify-content: space-between; }
.datagrid__chooser-heading span { font-variant-numeric: tabular-nums; }
.datagrid__chooser-list { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 160px), 1fr)); gap: 4px 12px; max-height: 210px; overflow: auto; }
.datagrid__chooser-list label { display: flex; align-items: baseline; gap: 6px; padding: 3px 0; cursor: pointer; overflow-wrap: anywhere; }
.datagrid__chooser-shortcuts { display: flex; flex-wrap: wrap; gap: 4px; margin-inline-end: auto; }
:deep(.datagrid__inspect) {font:inherit;color:inherit;background:transparent;border:0;padding:0;cursor:pointer;text-decoration:underline;}
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

.datagrid { container-type: inline-size; }
.datagrid__filter-toggle { display: none; }
@container (max-width: 600px) {
  .datagrid__filter-toggle { display: block; width: fit-content; padding: 4px 8px; font: inherit; color: inherit; background: transparent; border: 1px solid #8886; border-radius: 4px; cursor: pointer; }
  .datagrid__filter-toggle::before {
    content: ''; display: inline-block; margin-inline-end: 6px; vertical-align: 0.08em;
    border-block: 4px solid transparent; border-inline-start: 6px solid currentColor;
  }
  .datagrid__filter-toggle[aria-expanded=true]::before { transform: rotate(90deg); }
  .datagrid__filters:not(.datagrid__filters--open), .datagrid__filters-label { display: none; }
  .datagrid__filter { display: grid; width: 100%; grid-template-columns: minmax(0, 1fr) minmax(70px, 105px) auto; gap: 4px; }
  .datagrid__filter .n-select, .datagrid__filter .n-input { width: 100%; min-width: 0; }
  .datagrid__filter-value { grid-column: 1 / 3; grid-row: 2; }
  .datagrid__filter > button { grid-column: 3; grid-row: 1; }
}
</style>
