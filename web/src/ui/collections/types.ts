import type { VNodeChild } from 'vue'

export type RowKey = string | number
export type SortOrder = 'ascend' | 'descend'

export interface DataTableSortState {
  columnKey: string
  order: SortOrder
}

interface SelectionColumn {
  type: 'selection'
  width?: number
}

interface ValueColumn<Row> {
  type?: 'value'
  key: string
  title: string | (() => VNodeChild)
  width?: number
  fixed?: 'right'
  sorter?: 'default'
  sortOrder?: SortOrder | false
  cellProps?: (row: Row) => Record<string, unknown>
  render?: (row: Row) => VNodeChild
}

export type DataTableColumns<Row> = Array<SelectionColumn | ValueColumn<Row>>

export interface TreeOption {
  key: string
  label: string
  children?: TreeOption[]
  isLeaf?: boolean
  prefix?: () => VNodeChild
  suffix?: () => VNodeChild
}
