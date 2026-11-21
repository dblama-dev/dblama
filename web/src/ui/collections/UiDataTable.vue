<script setup lang="ts" generic="Row extends object">
import { computed, type VNodeChild } from 'vue'
import UiIcon from '../../components/UiIcon.vue'
import { useCollectionLabels } from './messages'
import type { DataTableColumns, DataTableSortState, RowKey } from './types'

const props = defineProps<{
  columns: DataTableColumns<Row>
  data: Row[]
  rowKey?: (row: Row) => RowKey
  rowProps?: (row: Row) => Record<string, unknown>
  checkedRowKeys?: RowKey[]
  scrollX?: number
  maxHeight?: string
  loading?: boolean
  size?: 'small'
}>()
const emit = defineEmits<{
  'update:checked-row-keys': [keys: RowKey[]]
  'update:sorter': [sort: DataTableSortState | null]
}>()
const { t } = useCollectionLabels()
const RenderContent = (props: { content: VNodeChild }) => props.content

const rows = computed(() => props.data.map((data, index) => ({
  data, key: props.rowKey ? props.rowKey(data) : index,
})))
const checked = computed(() => new Set(props.checkedRowKeys))
const selectedCount = computed(() => rows.value.filter(row => checked.value.has(row.key)).length)
const allSelected = computed(() => rows.value.length > 0 && selectedCount.value === rows.value.length)

function selectRows(keys: RowKey[], selected: boolean): void {
  const next = new Set(checked.value)
  for (const key of keys) {
    if (selected) next.add(key)
    else next.delete(key)
  }
  emit('update:checked-row-keys', [...next])
}

function sort(column: DataTableColumns<Row>[number]): void {
  if (column.type === 'selection' || !column.sorter) return
  if (column.sortOrder === 'ascend') emit('update:sorter', null)
  else emit('update:sorter', { columnKey: column.key, order: column.sortOrder === 'descend' ? 'ascend' : 'descend' })
}

function cellText(row: Row, key: string): string {
  const value = (row as Record<string, unknown>)[key]
  return value == null ? '' : String(value)
}
</script>

<template>
  <div class="ui-data-table" :aria-busy="!!loading" :class="{ 'ui-data-table--small': size === 'small' }">
    <div class="ui-data-table-wrapper" :style="{ maxHeight }">
      <table :inert="loading || undefined" :class="{ 'has-widths': columns.some(col => col.width) }" :style="{ minWidth: scrollX ? scrollX + 'px' : undefined }">
        <colgroup>
          <col v-for="(column, i) in columns" :key="i"
            :style="{ width: column.width ? column.width + 'px' : column.type === 'selection' ? '48px' : undefined }">
        </colgroup>
        <thead><tr>
          <template v-for="column in columns" :key="column.type === 'selection' ? 'select' : 'col:' + column.key">
            <th v-if="column.type === 'selection'" scope="col" class="ui-data-table-check">
              <input type="checkbox" :aria-label="t('selectPage')" :disabled="!rows.length || loading"
                :checked="allSelected" :indeterminate="selectedCount > 0 && !allSelected"
                @change="selectRows(rows.map(row => row.key), ($event.target as HTMLInputElement).checked)">
            </th>
            <th v-else scope="col" :class="{ 'fixed-end': column.fixed === 'right', sortable: column.sorter }"
              :aria-sort="column.sortOrder === 'ascend' ? 'ascending' : column.sortOrder === 'descend' ? 'descending' : undefined"
              @click="sort(column)">
              <button v-if="column.sorter" type="button" class="ui-data-sort">
                <RenderContent :content="typeof column.title === 'function' ? column.title() : column.title" />
                <UiIcon :name="column.sortOrder === 'ascend' ? 'arrow-up' : column.sortOrder === 'descend' ? 'arrow-down' : 'chevrons-up-down'" :size="12" />
              </button>
              <RenderContent v-else :content="typeof column.title === 'function' ? column.title() : column.title" />
            </th>
          </template>
        </tr></thead>
        <tbody>
          <tr v-for="(row, i) in rows" :key="row.key" v-bind="rowProps?.(row.data)" :class="{ selected: checked.has(row.key) }">
            <template v-for="column in columns" :key="column.type === 'selection' ? 'select' : 'col:' + column.key">
              <td v-if="column.type === 'selection'" class="ui-data-table-check">
                <input type="checkbox" :aria-label="t('selectRow', { number: i + 1 })"
                  :checked="checked.has(row.key)" :disabled="loading"
                  @change="selectRows([row.key], ($event.target as HTMLInputElement).checked)">
              </td>
              <td v-else v-bind="column.cellProps?.(row.data)" :class="{ 'fixed-end': column.fixed === 'right' }">
                <RenderContent :content="column.render ? column.render(row.data) : cellText(row.data, column.key)" />
              </td>
            </template>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-if="!rows.length" class="ui-table-status" role="status">{{ t(loading ? 'loading' : 'empty') }}</p>
    <span v-else-if="loading" class="ui-table-refresh" role="status">{{ t('loading') }}</span>
  </div>
</template>

<style scoped>
.ui-data-table { position: relative; min-width: 0; color: var(--db-ink); font-size: 13px; }
.ui-data-table-wrapper { width: 100%; overflow: auto; overscroll-behavior-x: contain; }
table { width: 100%; border-spacing: 0; }
table.has-widths { table-layout: fixed; }
th, td { box-sizing: border-box; padding: 9px 13px; text-align: start; vertical-align: middle; border-bottom: 1px solid var(--db-line); }
th { position: sticky; top: 0; z-index: 2; color: var(--db-muted); background: var(--db-sidebar); font-weight: 550; font-size: 12px; }
td { overflow-wrap: anywhere; background: var(--db-panel); }
tbody tr:last-child td { border-bottom: none; }
tbody tr:hover td { background: var(--db-sidebar); }
tbody tr.selected td { background: var(--db-tint); }
.ui-data-table--small :is(td, th) { padding: 6px 9px; }
.fixed-end { position: sticky; inset-inline-end: 0; z-index: 1; border-inline-start: 1px solid var(--db-line); }
th.fixed-end { z-index: 3; }
.ui-data-table-check { text-align: center; }
.ui-data-table-check input { vertical-align: middle; }
.sortable { cursor: pointer; }
.ui-data-sort { display: inline-flex; gap: 6px; align-items: center; max-width: 100%; padding: 0; text-align: start; font: inherit; color: inherit; border: 0; background: none; cursor: pointer; overflow-wrap: anywhere; }
.sortable:hover, .sortable[aria-sort] { color: var(--db-accent); }
.ui-table-status { margin: 0; padding: 26px 14px; color: var(--db-muted); text-align: center; }
.ui-table-refresh { position: absolute; top: 34px; inset-inline-end: 12px; z-index: 3; border: 1px solid var(--db-line); border-radius: 6px; background: var(--db-panel); padding: 2px 8px; color: var(--db-muted); font-size: 12px; }
</style>
