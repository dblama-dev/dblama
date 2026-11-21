<template>
  <div class="datagrid">
    <ui-alert v-if="!hasPk" type="warning" class="datagrid__no-pk">
      {{ t('grid.noPk') }}
    </ui-alert>

    <div class="datagrid__toolbar">
      <ui-button size="small" class="datagrid__filter-toggle" :type="filters.length ? 'primary' : 'default'" :secondary="!!filters.length"
        :aria-expanded="filtersOpen" :aria-controls="filterID" @click="filtersOpen = !filtersOpen">
        <template #icon><UiIcon name="filter" /></template>
        {{ t('grid.filters') }}<span v-if="filters.length">&nbsp;({{ filters.length }})</span>
      </ui-button>
      <ui-button ref="columnTrigger" size="small" data-testid="grid-columns"
        :aria-expanded="choosingColumns" :aria-controls="columnChooserID"
        :disabled="!columns.length" @click="toggleColumnChooser">
        <template #icon><UiIcon name="columns" /></template>
        {{ t('structure.columns') }}
        <span v-if="visibleMetadata.length !== columns.length" class="datagrid__column-count">{{ visibleMetadata.length }} / {{ columns.length }}</span>
      </ui-button>
      <ui-button size="small" class="datagrid__export" data-testid="grid-export"
        :title="t('rowWorkflows.exportVisible')" :disabled="loading || !visibleMetadata.length" @click="emit('export-selection')">
        <template #icon><UiIcon name="download" /></template>{{ t('rowWorkflows.csv') }}
      </ui-button>
      <ui-button size="small" :title="t('grid.refresh')" :aria-label="t('grid.refresh')" @click="emit('refresh')">
        <template #icon><UiIcon name="refresh" /></template>
      </ui-button>
    </div>

    <div v-if="checkedKeys.length" class="datagrid__selection">
      <ui-popconfirm @positive-click="onDeleteSelected">
        <template #trigger>
          <ui-button size="small" type="error">
            {{ t('grid.deleteSelected') }} ({{ checkedKeys.length }})
          </ui-button>
        </template>
        {{ t('grid.confirmDelete') }} {{ tableName }} ({{ checkedKeys.length }})
      </ui-popconfirm>
      <ui-popconfirm @positive-click="copySelected">
        <template #trigger><ui-button size="small">{{ t('grid.copySelected') }} ({{ checkedKeys.length }})</ui-button></template>
        {{ t('grid.copyConfirm') }}
      </ui-popconfirm>
      <ui-button size="small" :disabled="!canBulkEdit" @click="editSelected">{{ t('navigationWorkflows.editSelected') }} ({{ checkedKeys.length }})</ui-button>
      <small v-if="checkedKeys.length && !canBulkEdit" data-bulk-edit-unavailable>{{ t('navigationWorkflows.bulkNeedsPk') }}</small>
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
      <ui-input v-if="columns.length > 8" v-model:value="columnSearch" size="small" clearable :input-props="{ 'aria-label': t('rowWorkflows.findColumn') }" :placeholder="t('rowWorkflows.findColumn')" />
      <div class="datagrid__chooser-list">
        <label v-for="column in matchingColumns" :key="column.name">
          <input v-model="columnDraft" type="checkbox" :value="column.name">
          {{ column.name }}
        </label>
        <span v-if="!matchingColumns.length">{{ t('rowWorkflows.noMatchingColumns') }}</span>
      </div>
      <div class="datagrid__chooser-footer">
        <div class="datagrid__chooser-shortcuts">
          <ui-button size="small" quaternary data-testid="show-all-columns" :disabled="columnDraft.length === columns.length" @click="columnDraft = columns.map(col => col.name)">{{ t('rowWorkflows.showAllColumns') }}</ui-button>
          <ui-button size="small" quaternary data-testid="clear-columns" :disabled="!columnDraft.length" @click="columnDraft = []">{{ t('rowWorkflows.clearColumns') }}</ui-button>
        </div>
        <ui-button size="small" @click="closeColumnChooser">{{ t('row.cancel') }}</ui-button>
        <ui-button attr-type="submit" size="small" type="primary" data-testid="apply-columns" :disabled="!columnDraft.length">{{ t('grid.apply') }}</ui-button>
      </div>
    </form>
    <ui-alert v-if="columns.length && !visibleMetadata.length" type="warning">{{ t('rowWorkflows.columnsMissing') }}</ui-alert>

    <div :id="filterID" class="datagrid__filters" :hidden="!filtersOpen">
      <div v-for="(draft, i) in drafts" :key="i" class="datagrid__filter">
        <ui-select
          :aria-label="t('navigationWorkflows.filterColumn')"
          v-model:value="draft.column"
          size="small"
          clearable
          :placeholder="t('navigationWorkflows.filterColumn')"
          :options="columnOptions"
          class="datagrid__filter-column"
        />
        <ui-select
          :aria-label="t('navigationWorkflows.filterOperator')"
          v-model:value="draft.op"
          size="small"
          :options="operatorOptions"
          class="datagrid__filter-op"
        />
        <ui-input
          :aria-label="t('navigationWorkflows.filterValue')"
          @keydown.enter="applyFilters"
          v-model:value="draft.value"
          size="small"
          :disabled="isNullOperator(draft.op)"
          class="datagrid__filter-value"
        />
        <ui-button :aria-label="t('navigationWorkflows.removeFilter')" size="tiny" quaternary @click="removeDraft(i)">×</ui-button>
      </div>
      <div class="datagrid__filter-controls">
        <ui-button :aria-label="t('navigationWorkflows.addFilter')" size="small" quaternary @click="addDraft">
          <template #icon><UiIcon name="plus" :size="14" /></template>{{ t('navigationWorkflows.addFilter') }}
        </ui-button>
      <ui-button size="small" type="primary" @click="applyFilters">{{ t('grid.apply') }}</ui-button>
      <ui-button size="small" quaternary @click="resetFilters">{{ t('grid.reset') }}</ui-button>
      </div>
    </div>

    <div class="datagrid__surface">
    <ui-data-table v-if="!columns.length || visibleMetadata.length"
      size="small"
      :columns="gridColumns"
      :data="tableData"
      :loading="loading"
      :row-key="rowKey"
      :scroll-x="scrollWidth"
      max-height="max(240px, calc(100vh - 360px))"
      :checked-row-keys="checkedKeys"
      class="datagrid__table"
      @update:checked-row-keys="onCheckedRowKeys"
      @update:sorter="onSorter"
    />
      <footer class="datagrid__pagination">
        <span>{{ t('grid.total') }}: {{ totalRows.toLocaleString(locale) }}</span>
        <ui-pagination v-bind="pagination" :page-slot="pageSlots" @update:page="onPage" @update:page-size="onPageSize" />
      </footer>
    </div>
    <details v-if="statement" class="datagrid__statement">
      <summary><UiIcon name="code" :size="14" /> {{ t('grid.browseSQL') }}</summary>
      <pre tabindex="0">{{ statement.sql }}</pre>
      <span>{{ t('grid.parameters') }}</span> <output>{{ JSON.stringify(statement.parameters) }}</output>
    </details>

    <ui-modal v-model:show="inspectorShow" :title="`${t('grid.preview')}: ${inspectedColumn}`" style="width: min(800px, 96vw)">
      <ui-select v-if="inspectedRow" data-testid="grid-inspect-column" :value="inspectedColumn" :aria-label="t('structure.columns')" :options="columnOptions" @update:value="selectInspectedColumn" />
      <pre class="datagrid__full-value" data-testid="grid-inspected-value" tabindex="0" style="white-space: pre-wrap; overflow: auto; max-height: 70vh; overflow-wrap: anywhere">{{ inspectedValue }}</pre>
      <small v-if="inspectedRow?.[inspectedColumn] === ''">{{ t('rowWorkflows.emptyString') }}</small>
      <div v-if="inspectedFilter" class="datagrid__inspector-action">
        <ui-button size="small" data-testid="filter-inspected-value" :disabled="filtersEdited || alreadyFiltered || loading" @click="filterByInspectedValue">
          {{ t(alreadyFiltered ? 'rowWorkflows.alreadyFiltered' : 'rowWorkflows.filterByValue') }}
        </ui-button>
        <small v-if="filtersEdited">{{ t('rowWorkflows.unappliedFilters') }}</small>
      </div>
    </ui-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onBeforeUnmount, ref, useId, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { h } from 'vue'
import type { DataTableColumns, DataTableSortState, SelectOption } from '../ui'
import {
  UiAlert,
  UiButton,
  UiDataTable,
  UiInput,
  UiModal,
  UiPopconfirm,
  UiPagination,
  UiSelect,
} from '../ui'

import { useDatabaseMetadata } from '../stores/metadata'
import BlobValue from './BlobValue.vue'
import UiIcon from './UiIcon.vue'
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
    db: string
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
  'edit-row': [row: GridRow]
  'edit-rows': [keys: Record<string,unknown>[]]
  'copy-row': [row: GridRow]
  'copy-rows': [keys: Record<string,unknown>[]]
  'export-selection': []
  'delete-row': [pk: Record<string, unknown>]
  'delete-rows': [pks: Record<string, unknown>[]]
}>()

const { t, locale } = useI18n()
const metadata = useDatabaseMetadata(() => props.db)

const hasPk = computed(() => props.pkColumns.length > 0)

const visibleMetadata = computed(() => {
  if (!props.visibleColumns) return props.columns
  const selected = new Set(props.visibleColumns)
  return props.columns.filter(column => selected.has(column.name))
})
const choosingColumns = ref(false)
const columnChooserID = useId()
const columnTrigger = ref<InstanceType<typeof UiButton> | null>(null)
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

const compactViewport = window.matchMedia?.('(max-width: 719px), (pointer: coarse)')
const compactRows = ref(compactViewport?.matches ?? false)
const pageSlots = computed(() => compactRows.value ? 3 : 7)
const rowActionsWidth = computed(() => compactRows.value ? 50 : 144)
function updateViewport(): void { compactRows.value = compactViewport?.matches ?? false }
onMounted(() => compactViewport?.addEventListener('change', updateViewport))
onBeforeUnmount(() => compactViewport?.removeEventListener('change', updateViewport))

function columnWidth(meta: ColumnMeta): number {
  if (!compactRows.value) return Math.max(140, Math.min(300, meta.name.length * 9 + 70))
  const titleWidth = meta.name.length * 7.5 + (props.pkColumns.includes(meta.name) ? 58 : 38)
  let valueWidth = 52
  for (const row of tableData.value.slice(0, 25)) {
    const value = row[meta.name]
    const length = typeof value === 'object' && value !== null ? 24 : String(value ?? '').length
    valueWidth = Math.max(valueWidth, length * 7.5 + 18)
  }
  return Math.ceil(Math.min(230, Math.max(valueWidth, titleWidth)))
}

function cellClicked(event: MouseEvent, row: GridRow): void {
  if (!(event.target instanceof Element) || event.target.closest('button,input,a,select,textarea')) return
  if (!window.getSelection()?.isCollapsed && window.getSelection()?.toString()) return
  emit('edit-row', row)
}
const gridColumns = computed<DataTableColumns<GridRow>>(() => {
  const cols: DataTableColumns<GridRow> = []
  cols.push({ type: 'selection', width: compactRows.value ? 34 : 48 })
  for (const meta of visibleMetadata.value) {
    cols.push({
      width: columnWidth(meta),
      title: props.pkColumns.includes(meta.name)
        ? () => h('span', { class: 'datagrid__column-key' }, [h(UiIcon, { name: 'key', size: 12 }), meta.name])
        : meta.name,
      key: meta.name,
      sorter: 'default',
      sortOrder:
        props.sort?.column === meta.name ? (props.sort.desc ? 'descend' : 'ascend') : false,
      cellProps: (row) => ({
        'aria-label': cellPreview(cellText(row[meta.name])),
        onClick: (event: MouseEvent) => cellClicked(event, row),
      }),
      render: (row) => renderCell(row, meta.name),
    })
  }
  cols.push({
    title: compactRows.value ? '' : t('grid.actions'),
    key: '__actions',
    width: rowActionsWidth.value,
    fixed: 'right',
    render: row => h(GridRowActions, {
      compact: compactRows.value,
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
const filtersOpen = ref(false)
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
const scrollWidth = computed(() => gridColumns.value.reduce((sum, column) => sum + (column.width ?? 0), 0))

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
  // This grid supports one server-side sort. An empty sort restores the default order.
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
.datagrid { display:flex; min-width:0; flex-direction:column; gap:12px; container-type:inline-size; }
.datagrid__toolbar, .datagrid__selection, .datagrid__filter-controls, .datagrid__inspector-action, .datagrid__chooser-footer, .datagrid__chooser-heading { display:flex; flex-wrap:wrap; align-items:center; gap:8px; }
.datagrid__toolbar { gap:6px; }
.datagrid__export { margin-inline-start:auto; }
.datagrid__selection, .datagrid__chooser, .datagrid__filters { padding:14px; border-radius:9px; background:var(--db-sidebar); border:1px solid var(--db-line); }
.datagrid__selection { background:var(--db-tint); }
.datagrid__chooser, .datagrid__filters { display:grid; gap:12px; }
.datagrid__filters[hidden] { display:none; }
.datagrid__column-count { padding-inline-start:5px; font-variant-numeric:tabular-nums; }
.datagrid__chooser-heading { justify-content:space-between; }
.datagrid__chooser-heading span { color:var(--db-muted); font-size:12px; }
.datagrid__chooser-list { display:grid; grid-template-columns:repeat(auto-fill,minmax(min(100%,160px),1fr)); gap:5px 12px; max-height:210px; overflow:auto; }
.datagrid__chooser-list label { display:flex; align-items:center; gap:7px; cursor:pointer; overflow-wrap:anywhere; font:13px var(--db-mono); }
.datagrid__chooser-shortcuts { display:flex; flex-wrap:wrap; gap:4px; margin-inline-end:auto; }
.datagrid__filter { display:grid; grid-template-columns:minmax(100px,1fr) minmax(90px,.55fr) minmax(120px,1.4fr) auto; align-items:center; gap:8px; }
.datagrid__filter .ui-select, .datagrid__filter .ui-input { min-width:0; }
.datagrid__filter-controls > :first-child { margin-inline-end:auto; }
.datagrid__surface { min-width:0; border:1px solid var(--db-line); border-radius:10px; overflow:hidden; background:var(--db-panel); }
.datagrid__table { max-width:100%; }
.datagrid__table :deep(td[aria-label]) { cursor:pointer; }
@media (hover:hover) and (pointer:fine) {
  .datagrid__table :deep(tr:not(:hover):not(:focus-within) .row-actions .secondary-action) { opacity:0; }
}
.datagrid__table :deep(.ui-data-table-wrapper) { border:0; border-radius:0; }
.datagrid__table :deep(th:not(:last-child)) { font-family:var(--db-mono); font-size:12px; }
.datagrid__table :deep(.datagrid__column-key) { display:inline-flex; align-items:center; gap:5px; }
.datagrid__table :deep(.datagrid__column-key svg) { color:var(--db-muted); flex-shrink:0; }
.datagrid__table :deep(.datagrid__null) { color:var(--db-muted); font:italic 12px var(--db-mono); }
.datagrid__table :deep(.datagrid__inspect) { padding:2px 4px; border:0; border-radius:4px; background:var(--db-tint); color:var(--db-accent); font:inherit; font-size:11px; cursor:pointer; }
.datagrid__pagination { display:flex; flex-wrap:wrap; align-items:center; justify-content:space-between; gap:10px; min-height:45px; padding:8px 14px; border-top:1px solid var(--db-line); font-size:12px; color:var(--db-muted); }
.datagrid__statement { margin:0; font-size:12px; color:var(--db-muted); }
.datagrid__statement summary { width:fit-content; cursor:pointer; }
.datagrid__statement summary svg { vertical-align:middle; margin:0 4px 2px; }
.datagrid__statement pre,.datagrid__full-value { padding:12px 14px; margin:8px 0; border-radius:7px; border:1px solid var(--db-line); background:var(--db-sidebar); color:var(--db-ink); font-family:var(--db-mono); white-space:pre-wrap; overflow-wrap:anywhere; }
.datagrid__statement output { font-family:var(--db-mono); overflow-wrap:anywhere; }
.datagrid__inspector-action { margin-top:10px; }
@container (max-width:540px) {
  .datagrid__toolbar { row-gap:8px; }
  .datagrid__export { margin-inline-start:0; }
  .datagrid__chooser, .datagrid__filters { padding:10px; }
  .datagrid__filter { grid-template-columns:minmax(0,1fr) minmax(70px,100px) auto; gap:6px; }
  .datagrid__filter-value { grid-column:1 / 3; grid-row:2; }
  .datagrid__filter > button { grid-column:3; grid-row:1; }
  .datagrid__pagination { padding:8px; gap:6px; }
}
@container (max-width:440px) {
  .datagrid__chooser-list { row-gap:2px; }
  .datagrid__chooser-list label { min-height:30px; }
  .datagrid__chooser-footer { justify-content:flex-end; }
  .datagrid__chooser-shortcuts { flex-basis:100%; }
}
@media (max-width:719px), (pointer:coarse) {
  .datagrid__table :deep(td.fixed-end) { padding:1px 3px; text-align:center; }
  .datagrid__table :deep(.row-actions .row-action) { width:42px; height:42px; }
}
</style>
