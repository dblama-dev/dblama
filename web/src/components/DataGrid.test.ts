import { createPinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import {
  NDataTable,
  NInput,
  NNotificationProvider,
  NPopconfirm,
  NSelect,
} from 'naive-ui'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api, ApiError } from '../api/client'
import type { ColumnMeta, Page } from '../api/types'
import i18n from '../i18n'
import DataGrid from './DataGrid.vue'
import TableView from '../views/TableView.vue'

vi.mock('../api/client', () => ({
  api: vi.fn(),
  ApiError: class ApiError extends Error {
    constructor(
      public code: string,
      message: string,
      public status: number,
    ) {
      super(message)
      this.name = 'ApiError'
    }
  },
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { db: 'mydb', t: 'users' }, query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

const mockedApi = vi.mocked(api)

const columns: ColumnMeta[] = [
  { name: 'id', type: 'int', nullable: false },
  { name: 'name', type: 'varchar', nullable: true },
]

const rows: unknown[][] = [
  [1, 'alice'],
  [2, null],
]

const page: Page = { columns, rows, totalRows: 2 }

function mountGrid(props: Record<string, unknown> = {}) {
  return mount(DataGrid, {
    props: {
      columns,
      rows,
      totalRows: 2,
      loading: false,
      pageIndex: 1,
      pageSize: 50,
      pkColumns: ['id'],
      sort: null,
      ...props,
    },
    global: { plugins: [createPinia(), i18n] },
  })
}

describe('DataGrid', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders the columns and cells of the page', () => {
    const wrapper = mountGrid()

    const headers = wrapper.findAll('th').map((th) => th.text())
    expect(headers).toContain('id')
    expect(headers).toContain('name')
    const text = wrapper.text()
    expect(text).toContain('alice')
    expect(text).toContain('1')
  })

  it('renders NULL cells as an italic NULL marker', () => {
    const wrapper = mountGrid()

    const nulls = wrapper.findAll('.datagrid__null')
    expect(nulls).toHaveLength(1)
    expect(nulls[0].text()).toBe('NULL')
    expect(nulls[0].attributes('style')).toContain('italic')
  })

  it('renders hex strings in monospace with the full value in the title', () => {
    const wrapper = mountGrid({ rows: [['0xDEADBEEF', 'x']] })

    const hex = wrapper.find('.datagrid__hex')
    expect(hex.exists()).toBe(true)
    expect(hex.attributes('title')).toBe('0xDEADBEEF')
  })

  it('re-emits pagination page and page-size changes', async () => {
    const wrapper = mountGrid()

    const table = wrapper.findComponent(NDataTable)
    table.vm.$emit('update:page', 3)
    table.vm.$emit('update:page-size', 100)
    await flushPromises()

    expect(wrapper.emitted('update:page')).toEqual([[3]])
    expect(wrapper.emitted('update:page-size')).toEqual([[100]])
  })

  it('maps the remote sorter event to a sort state', async () => {
    const wrapper = mountGrid()
    const table = wrapper.findComponent(NDataTable)

    table.vm.$emit('update:sorter', { column: { key: 'name' }, order: 'descend' })
    await flushPromises()
    expect(wrapper.emitted('update:sort')).toEqual([[{ column: 'name', desc: true }]])

    table.vm.$emit('update:sorter', { column: { key: 'name' }, order: 'ascend' })
    await flushPromises()
    expect(wrapper.emitted('update:sort')).toEqual([
      [{ column: 'name', desc: true }],
      [{ column: 'name', desc: false }],
    ])

    table.vm.$emit('update:sorter', { column: { key: 'name' }, order: false })
    await flushPromises()
    expect(wrapper.emitted('update:sort')?.[2]).toEqual([null])
  })

  it('emits parsed pk maps for the selected rows on delete-selected', async () => {
    const wrapper = mountGrid()

    const table = wrapper.findComponent(NDataTable)
    table.vm.$emit('update:checked-row-keys', ['1', '2'], [], {})
    await flushPromises()

    const popconfirm = wrapper.findComponent(NPopconfirm)
    expect(popconfirm.exists()).toBe(true)
    popconfirm.vm.$emit('positive-click')
    await flushPromises()

    expect(wrapper.emitted('delete-rows')).toEqual([[{ id: 1 }, { id: 2 }]].map((pks) => [pks]))
  })

  it('emits the pk map on a single-row delete', async () => {
    const wrapper = mountGrid()

    const deleteButtons = wrapper.findAll('button').filter((b) => b.text() === 'Удалить')
    expect(deleteButtons.length).toBe(2)
    await deleteButtons[1].trigger('click')
    await flushPromises()

    expect(wrapper.emitted('delete-row')).toEqual([[{ id: 2 }]])
  })

  it('emits add-row from the toolbar', async () => {
    const wrapper = mountGrid()

    const add = wrapper.findAll('button').find((b) => b.text() === 'Добавить строку')
    expect(add).toBeDefined()
    await add?.trigger('click')

    expect(wrapper.emitted('add-row')).toHaveLength(1)
  })

  it('collects the filter drafts into filters on apply and resets them', async () => {
    const wrapper = mountGrid()
    const filtersEl = wrapper.find('.datagrid__filters')
    const selects = filtersEl.findAllComponents(NSelect)
    expect(selects.length).toBeGreaterThanOrEqual(2)

    selects[0].vm.$emit('update:value', 'name')
    selects[1].vm.$emit('update:value', 'LIKE')
    filtersEl.findComponent(NInput).vm.$emit('update:value', '%a%')
    await flushPromises()

    const apply = wrapper.findAll('button').find((b) => b.text() === 'Применить')
    await apply?.trigger('click')
    await flushPromises()

    expect(wrapper.emitted('apply-filters')).toEqual([[[{ column: 'name', op: 'LIKE', value: '%a%' }]]])

    const reset = wrapper.findAll('button').find((b) => b.text() === 'Сбросить')
    await reset?.trigger('click')
    await flushPromises()
    expect(wrapper.emitted('apply-filters')?.[1]).toEqual([[]])
  })

  it('disables the value input for IS NULL operators', async () => {
    const wrapper = mountGrid()
    const filtersEl = wrapper.find('.datagrid__filters')

    filtersEl.findAllComponents(NSelect)[1].vm.$emit('update:value', 'IS NULL')
    await flushPromises()

    expect(filtersEl.findComponent(NInput).props('disabled')).toBe(true)
  })

  it('hides selection, row actions and shows a warning when the table has no pk', async () => {
    const wrapper = mountGrid({ pkColumns: [] })

    expect(wrapper.text()).toContain('У таблицы нет первичного ключа')
    expect(wrapper.findComponent(NDataTable).props('rowKey')).toBeDefined()
    // No checkbox column: no checkbox rendered anywhere in the grid.
    expect(wrapper.findAll('.n-checkbox')).toHaveLength(0)
    // No edit/delete buttons in rows.
    const buttons = wrapper.findAll('button').map((b) => b.text())
    expect(buttons).not.toContain('Изменить')
    expect(buttons).not.toContain('Удалить')
  })
})

describe('TableView data view', () => {
  const pkUrl = '/api/databases/mydb/tables/users/pk'
  const rowsUrl = '/api/databases/mydb/tables/users/rows'

  let deleteBodies: unknown[]
  let lastRowsUrl: string

  function mockApi(rowsPayload: Page, pk: { columns: string[] }): void {
    deleteBodies = []
    lastRowsUrl = ''
    mockedApi.mockImplementation(async (method, url, body) => {
      if (method === 'DELETE' && url === rowsUrl) {
        deleteBodies.push(body)
        return undefined
      }
      if (url === pkUrl) return pk
      if (typeof url === 'string' && url.startsWith(rowsUrl)) {
        lastRowsUrl = url
        return rowsPayload
      }
      throw new Error(`unexpected api call: ${method} ${url}`)
    })
  }

  function queryParams(): URLSearchParams {
    return new URLSearchParams(lastRowsUrl.split('?')[1] ?? '')
  }

  const TableViewInProvider = defineComponent({
    setup() {
      return () => h(NNotificationProvider, null, { default: () => h(TableView) })
    },
  })

  async function mountView(): ReturnType<typeof mount> {
    const wrapper = mount(TableViewInProvider, { global: { plugins: [createPinia(), i18n] } })
    await flushPromises()
    await flushPromises()
    return wrapper
  }

  beforeEach(() => {
    mockApi(page, { columns: ['id'] })
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('fetches pk and the first rows page on mount', async () => {
    await mountView()

    expect(mockedApi).toHaveBeenCalledWith('GET', pkUrl)
    expect(queryParams().get('limit')).toBe('50')
    expect(queryParams().get('offset')).toBe('0')
  })

  it('refetches with the new offset when the page changes', async () => {
    const wrapper = await mountView()

    const grid = wrapper.findComponent(DataGrid)
    grid.vm.$emit('update:page', 2)
    await flushPromises()
    await flushPromises()

    expect(queryParams().get('offset')).toBe('50')
  })

  it('refetches with an order param when a column is sorted', async () => {
    const wrapper = await mountView()

    const grid = wrapper.findComponent(DataGrid)
    grid.vm.$emit('update:sort', { column: 'name', desc: true })
    await flushPromises()
    await flushPromises()

    expect(queryParams().get('order')).toBe('name:desc')
  })

  it('sends the applied filters with the next fetch', async () => {
    const wrapper = await mountView()

    const grid = wrapper.findComponent(DataGrid)
    grid.vm.$emit('apply-filters', [{ column: 'name', op: 'LIKE', value: '%a%' }])
    await flushPromises()
    await flushPromises()

    expect(queryParams().getAll('filter')).toEqual(['name:LIKE:%a%'])
  })

  it('deletes selected rows and refetches the current page', async () => {
    const wrapper = await mountView()

    const grid = wrapper.findComponent(DataGrid)
    grid.vm.$emit('delete-rows', [{ id: 1 }, { id: 2 }])
    await flushPromises()
    await flushPromises()

    expect(deleteBodies).toEqual([{ where: [{ id: 1 }, { id: 2 }] }])
    // Two initial fetches (pk + rows) then one refetch after delete.
    const rowCalls = mockedApi.mock.calls.filter(([m, u]) => m === 'GET' && String(u).startsWith(rowsUrl))
    expect(rowCalls.length).toBe(2)
  })

  it('steps back one page after a delete empties the current page', async () => {
    const wrapper = await mountView()

    const grid = wrapper.findComponent(DataGrid)
    grid.vm.$emit('update:page', 2)
    await flushPromises()
    await flushPromises()
    expect(queryParams().get('offset')).toBe('50')

    // The last page holds a single row; deleting it leaves the page empty.
    mockedApi.mockImplementation(async (method, url, body) => {
      if (method === 'DELETE' && url === rowsUrl) {
        deleteBodies.push(body)
        return undefined
      }
      if (url === pkUrl) return { columns: ['id'] }
      if (typeof url === 'string' && url.startsWith(rowsUrl)) {
        lastRowsUrl = url
        return { columns, rows: [], totalRows: 51 }
      }
      throw new Error(`unexpected api call: ${method} ${url}`)
    })

    grid.vm.$emit('delete-row', { id: 51 })
    await flushPromises()
    await flushPromises()

    expect(queryParams().get('offset')).toBe('0')
  })

  it('passes empty pkColumns and shows the warning for a table without a pk', async () => {
    mockApi(page, { columns: [] })
    const wrapper = await mountView()

    const grid = wrapper.findComponent(DataGrid)
    expect(grid.props('pkColumns')).toEqual([])
    expect(wrapper.text()).toContain('У таблицы нет первичного ключа')
  })

  it('shows an error notification when the rows fetch fails', async () => {
    mockedApi.mockImplementation(async (_method, url) => {
      if (url === pkUrl) return { columns: ['id'] }
      throw new ApiError('bad_request', 'invalid filter parameter', 400)
    })

    const wrapper = await mountView()

    // n-notification-provider teleports notifications to document.body.
    expect(document.body.textContent).toContain('invalid filter parameter')
    wrapper.unmount()
  })
})
