import { createPinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import {
  UiDataTable,
  UiInput,
  UiNotificationProvider,
  UiPopconfirm,
  UiSelect,
} from '../ui'
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

function mountGrid(props: Record<string, unknown> = {}, attachTo?: HTMLElement) {
  return mount(DataGrid, {
    attachTo,
    props: {
      db: 'mydb',
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

  it('applies a column selection while retaining the hidden primary key and row values for edits', async () => {
    const wrapper = mountGrid({}, document.body)
    await wrapper.get('[data-testid="grid-columns"]').trigger('click')
    const chooser = wrapper.get('[data-testid="grid-column-chooser"]')
    await chooser.get('input[value="id"]').setValue(false)
    await chooser.get('input[value="name"]').setValue(false)
    expect(chooser.get('[data-testid="apply-columns"]').attributes('disabled')).toBeDefined()
    await chooser.get('input[value="name"]').setValue(true)
    await chooser.get('[data-testid="apply-columns"]').trigger('click')
    expect(wrapper.emitted('update:visible-columns')).toEqual([[['name']]])

    await wrapper.setProps({ visibleColumns: ['name'] })
    expect(wrapper.findAll('th').map(header => header.text())).not.toContain('id')
    expect(wrapper.findAll('tbody tr').at(0)?.findAll('td').at(1)?.text()).toBe('alice')
    const edit = wrapper.get('tbody tr').findAll('button').find(button => button.attributes('aria-label') === 'Изменить')!
    await edit.trigger('click')
    expect(wrapper.emitted('edit-row')?.[0]?.[0]).toMatchObject({ id: 1, name: 'alice', __pk: { id: 1 } })
    wrapper.unmount()
  })

  it('can restore all columns when none of the linked column names still exists', async () => {
    const wrapper = mountGrid({ visibleColumns: [] }, document.body)
    expect(wrapper.get('[data-testid="grid-export"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="grid-columns"]').trigger('click')
    const chooser = wrapper.get('[data-testid="grid-column-chooser"]')
    await chooser.get('[data-testid="show-all-columns"]').trigger('click')
    await chooser.get('[data-testid="apply-columns"]').trigger('click')
    expect(wrapper.emitted('update:visible-columns')).toEqual([[['id', 'name']]])
    wrapper.unmount()
  })

  it('selects a few fields in a wide table across separate searches and discards cancelled changes', async () => {
    const wideColumns = Array.from({ length: 40 }, (_, i) => ({ name: `field ${i}`, type: 'text', nullable: false }))
    const wrapper = mountGrid({ columns: wideColumns, rows: [], pkColumns: [] }, document.body)
    const trigger = wrapper.get('[data-testid="grid-columns"]')
    await trigger.trigger('click')
    const chooser = wrapper.get('[data-testid="grid-column-chooser"]')
    await chooser.get('[data-testid="clear-columns"]').trigger('click')
    const search = chooser.get('input:not([type=checkbox])')
    await search.setValue('FIELD 35')
    expect(chooser.findAll('input[type=checkbox]')).toHaveLength(1)
    await chooser.get('input[type=checkbox][value="field 35"]').setValue(true)
    await search.setValue('field 10')
    await chooser.get('input[type=checkbox][value="field 10"]').setValue(true)
    await chooser.get('[data-testid="apply-columns"]').trigger('click')
    expect(wrapper.emitted('update:visible-columns')).toEqual([[['field 10', 'field 35']]])
    await wrapper.setProps({ visibleColumns: ['field 10', 'field 35'] })
    await trigger.trigger('click')
    await wrapper.get('[data-testid="show-all-columns"]').trigger('click')
    await wrapper.get('[data-testid="grid-column-chooser"]').trigger('keydown.esc')
    expect(wrapper.emitted('update:visible-columns')).toHaveLength(1)
    expect(document.activeElement).toBe(trigger.element)
    wrapper.unmount()
  })

  it('opens a record from its cell and keeps the inspection control independent', async () => {
    const wrapper = mountGrid({ rows: [[1, 'Long value '.repeat(20)]] })
    await wrapper.get('.datagrid__cell-short').trigger('click')
    expect(wrapper.emitted('edit-row')).toHaveLength(1)
    await wrapper.get('.datagrid__inspect').trigger('click')
    expect(wrapper.emitted('edit-row')).toHaveLength(1)
    wrapper.unmount()
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
    const wrapper = mountGrid({ totalRows: 150 })
    await wrapper.get('.ui-pagination button[aria-label="Страница 3"]').trigger('click')
    await wrapper.get('.ui-pagination select').setValue('100')

    expect(wrapper.emitted('update:page')).toEqual([[3]])
    expect(wrapper.emitted('update:page-size')).toEqual([[100]])
  })

  it('cycles remote sort state when a user clicks a column header', async () => {
    const wrapper = mountGrid()
    const nameHeader = () => wrapper.findAll('th').find(header => header.text() === 'name')!

    await nameHeader().trigger('click')
    await flushPromises()
    expect(wrapper.emitted('update:sort')).toEqual([[{ column: 'name', desc: true }]])
    await wrapper.setProps({ sort: { column: 'name', desc: true } })

    await nameHeader().trigger('click')
    await flushPromises()
    expect(wrapper.emitted('update:sort')).toEqual([
      [{ column: 'name', desc: true }],
      [{ column: 'name', desc: false }],
    ])
    await wrapper.setProps({ sort: { column: 'name', desc: false } })

    await nameHeader().trigger('click')
    await flushPromises()
    expect(wrapper.emitted('update:sort')?.[2]).toEqual([null])
  })

  it('emits parsed pk maps for the selected rows on delete-selected', async () => {
    const wrapper = mountGrid()

    const table = wrapper.findComponent(UiDataTable)
    table.vm.$emit('update:checked-row-keys', ['1', '2'], [], {})
    await flushPromises()

    const popconfirm = wrapper.findComponent(UiPopconfirm)
    expect(popconfirm.exists()).toBe(true)
    popconfirm.vm.$emit('positive-click')
    await flushPromises()

    expect(wrapper.emitted('delete-rows')).toEqual([[{ id: 1 }, { id: 2 }]].map((pks) => [pks]))
  })

  it('emits the pk map on a single-row delete', async () => {
    const wrapper = mountGrid()

    const deleteButtons = wrapper.findAll('button').filter((b) => b.attributes('aria-label') === 'Удалить')
    expect(deleteButtons.length).toBe(2)
    await deleteButtons[1].trigger('click')
    await flushPromises()

    expect(wrapper.emitted('delete-row')).toBeUndefined()
    wrapper.findAllComponents(UiPopconfirm)[1].vm.$emit('positive-click')
    await flushPromises()
    expect(wrapper.emitted('delete-row')).toEqual([[{ id: 2 }]])
  })

  it('opens and closes filter controls without discarding an unfinished value', async () => {
    const wrapper = mountGrid()
    const trigger = wrapper.get('.datagrid__filter-toggle')
    expect(trigger.attributes('aria-expanded')).toBe('false')
    await trigger.trigger('click')
    expect(trigger.attributes('aria-expanded')).toBe('true')
    const panel = wrapper.get('.datagrid__filters')
    panel.findComponent(UiInput).vm.$emit('update:value', 'keep me')
    await trigger.trigger('click')
    expect(panel.attributes('hidden')).toBeDefined()
    await trigger.trigger('click')
    expect(panel.findComponent(UiInput).props('value')).toBe('keep me')
    wrapper.unmount()
  })

  it('collects the filter drafts into filters on apply and resets them', async () => {
    const wrapper = mountGrid()
    const filtersEl = wrapper.find('.datagrid__filters')
    const selects = filtersEl.findAllComponents(UiSelect)
    expect(selects.length).toBeGreaterThanOrEqual(2)

    selects[0].vm.$emit('update:value', 'name')
    selects[1].vm.$emit('update:value', 'LIKE')
    filtersEl.findComponent(UiInput).vm.$emit('update:value', '%a%')
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

    filtersEl.findAllComponents(UiSelect)[1].vm.$emit('update:value', 'IS NULL')
    await flushPromises()

    expect(filtersEl.findComponent(UiInput).props('disabled')).toBe(true)
  })

  it('uses all original values for row actions when there is no primary key', async () => {
    const wrapper = mountGrid({ pkColumns: [] })
    expect(wrapper.text()).toContain('У таблицы нет первичного ключа')
    const edits = wrapper.findAll('button').filter(b => b.attributes('aria-label') === 'Изменить')
    await edits[1]!.trigger('click')
    expect(wrapper.emitted('edit-row')?.[0]?.[0]).toMatchObject({ __pk: {id:2,name:null} })
    const deletes = wrapper.findAll('button').filter(b => b.attributes('aria-label') === 'Удалить')
    await deletes[1]!.trigger('click')
    expect(wrapper.emitted('delete-row')).toBeUndefined()
    wrapper.findAllComponents(UiPopconfirm)[1].vm.$emit('positive-click')
    await flushPromises()
    expect(wrapper.emitted('delete-row')).toEqual([[{id:2,name:null}]])
    wrapper.unmount()
  })
  it('offers copying a row and exporting the current query', async () => {
    const wrapper = mountGrid()
    await wrapper.get('[data-testid=grid-export]').trigger('click')
    expect(wrapper.emitted('export-selection')).toHaveLength(1)
    await wrapper.findAll('[data-testid=copy-row]')[0]!.trigger('click')
    expect(wrapper.emitted('copy-row')?.[0]?.[0]).toMatchObject({id:1,name:'alice'})
    wrapper.unmount()
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
      return () => h(UiNotificationProvider, null, { default: () => h(TableView) })
    },
  })

  async function mountView(): Promise<ReturnType<typeof mount>> {
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

it('restores the visible controlled filter and applies it with Enter',async()=>{
 const wrapper=mountGrid({filters:[{column:'name',op:'LIKE',value:'%restored%'}]})
 expect(wrapper.findComponent(UiInput).props('value')).toBe('%restored%')
 await wrapper.get('.datagrid__filter-value input').trigger('keydown.enter')
 expect(wrapper.emitted('apply-filters')).toEqual([[[{column:'name',op:'LIKE',value:'%restored%'}]]])
 wrapper.unmount()
})
it('clears captured selection when rows are replaced in a keyless table',async()=>{
 const wrapper=mountGrid({pkColumns:[]})
 await wrapper.get('tbody input[type=checkbox]').setValue(true)
 expect(wrapper.get<HTMLInputElement>('thead input[type=checkbox]').element.indeterminate).toBe(true)
 await wrapper.setProps({rows:[[9,'different']]})
 expect(wrapper.get<HTMLInputElement>('tbody input[type=checkbox]').element.checked).toBe(false)
 expect(wrapper.get<HTMLInputElement>('thead input[type=checkbox]').element.indeterminate).toBe(false)
 wrapper.unmount()
})

it('disables bulk editing of one selected duplicate in a keyless table', async () => {
 const wrapper = mountGrid({ pkColumns: [], rows: [[1, 'same'], [1, 'same']] })
 wrapper.findComponent(UiDataTable).vm.$emit('update:checked-row-keys', ['__idx_0'])
 await flushPromises()
 const edit = wrapper.findAll('button').find(button => button.text().startsWith('Изменить выбранные'))!
 expect(edit.attributes('disabled')).toBeDefined()
 expect(wrapper.get('[data-bulk-edit-unavailable]').text()).toContain('первичный ключ')
 await edit.trigger('click')
 expect(wrapper.emitted('edit-rows')).toBeUndefined()
 wrapper.unmount()
})

it('emits only complete composite primary keys for a captured bulk selection', async () => {
 const wrapper = mountGrid({ pkColumns: ['id', 'name'] })
 wrapper.findComponent(UiDataTable).vm.$emit('update:checked-row-keys', ['[1,"alice"]'])
 await flushPromises()
 await wrapper.findAll('button').find(button => button.text().startsWith('Изменить выбранные'))!.trigger('click')
 expect(wrapper.emitted('edit-rows')).toEqual([[[{ id: 1, name: 'alice' }]]])
 wrapper.unmount()
})

it('keeps short cell values as plain, exact text with meaningful cell names', () => {
  const wrapper = mountGrid({ rows: [['9007199254740993', '12345678901234567890.123456'], [2, '']] })
  const cells = wrapper.findAll('tbody tr')[0]!.findAll('td')
  expect(cells[1]!.text()).toBe('9007199254740993')
  expect(cells[2]!.text()).toBe('12345678901234567890.123456')
  expect(cells[1]!.find('details').exists()).toBe(false)
  expect(cells[1]!.attributes('aria-label')).toBe('9007199254740993')
  expect(cells[2]!.find('pre').exists()).toBe(false)
  expect(wrapper.findAll('tbody tr')[1]!.findAll('td')[2]!.text()).toBe('')
  wrapper.unmount()
})

it('bounds long previews and opens the exact full value outside the cell', async () => {
  const value = 'Long Unicode value 世界\n'.repeat(40)
  const wrapper = mountGrid({ rows: [[1, value]] })
  const cell = wrapper.find('tbody tr')!.findAll('td')[2]!
  expect(cell.text().length).toBeLessThan(160)
  expect(cell.find('pre').exists()).toBe(false)
  expect(cell.attributes('aria-label')).toContain('Long Unicode value')
  const preview = cell.find('button')
  expect(preview.attributes('aria-label')).toContain('name')
  await preview.trigger('click')
  await flushPromises()
  const full = document.body.querySelector('[data-testid="grid-inspected-value"]')
  expect(full?.textContent).toBe(value)
  expect(cell.text().length).toBeLessThan(160)
  wrapper.unmount()
})

it('applies compact overflow bounds to the actual rendered cell nodes', async () => {
  const wrapper = mountGrid({ rows: [[1, 'Long notes with newlines\n'.repeat(15)]] })
  const short = wrapper.get('.datagrid__cell-short').element
  const preview = wrapper.get('.datagrid__cell-preview > span').element
  expect(getComputedStyle(short).whiteSpace).toBe('nowrap')
  expect(getComputedStyle(short).overflow).toBe('hidden')
  expect(getComputedStyle(preview).whiteSpace).toBe('nowrap')
  expect(getComputedStyle(preview).overflow).toBe('hidden')
  expect(wrapper.get('.datagrid__cell-preview > span').text()).not.toContain('\n')
  await wrapper.get('.datagrid__inspect').trigger('click')
  await flushPromises()
  const full = document.body.querySelector('[data-testid="grid-inspected-value"]')!
  expect(Number.parseFloat(getComputedStyle(full).maxHeight)).toBeCloseTo(window.innerHeight * 0.7)
  expect(getComputedStyle(full).overflow).toBe('auto')
  wrapper.unmount()
})

it('offers keyboard-operable row inspection for clipped values below 120 characters', async () => {
  const value = 'https://example.test/a/long/path/that/exceeds/a/narrow/cell'
  const wrapper = mountGrid({ rows: [[1, value]] })
  const row = wrapper.get('tbody tr')
  expect(row.findAll('td')[1]!.text()).toBe('1')
  expect(row.findAll('td')[2]!.text()).toBe(value)
  await row.get('[data-testid="row-inspect"]').trigger('click')
  await flushPromises()
  const picker = wrapper.findAllComponents(UiSelect).find(select => select.attributes('data-testid') === 'grid-inspect-column')!
  picker.vm.$emit('update:value', 'name')
  await flushPromises()
  expect(document.body.querySelector('[data-testid="grid-inspected-value"]')?.textContent).toBe(value)
  expect(row.findAll('td')[2]!.text()).toBe(value)
  wrapper.unmount()
})
it('keeps the table body viewport bounded so its horizontal scrollbar stays reachable', () => {
  const wrapper = mountGrid({ rows: Array.from({length:50}, (_, i) => [i, 'row']) })
  const expected = document.createElement('div')
  expected.style.maxHeight = 'max(240px, calc(100vh - 360px))'
  expect(wrapper.get<HTMLElement>('.ui-data-table-wrapper').element.style.maxHeight).toBe(expected.style.maxHeight)
  wrapper.unmount()
})

describe('filtering from the value inspector', () => {
  const button = () => document.body.querySelector<HTMLButtonElement>('[data-testid="filter-inspected-value"]')

  it.each([
    ['varchar', '  A:%_  ', '=', '  A:%_  '],
    ['text', '', '=', ''],
    ['bytea', null, 'IS NULL', ''],
    ['numeric', '12345678901234567890.10001', '=', '12345678901234567890.10001'],
    ['boolean', false, '=', 'false'],
  ])('adds a literal condition for %s value %j to the applied filters', async (type, value, op, expected) => {
    const existing = { column: 'id', op: '>', value: '4' }
    const wrapper = mountGrid({ columns: [{ name: 'data', type, nullable: true }, columns[0]], rows: [[value, 8]], filters: [existing] })
    try {
      await wrapper.get('[data-testid="row-inspect"]').trigger('click')
      await flushPromises()
      expect(button()?.disabled).toBe(false)
      button()!.click()
      expect(wrapper.emitted('apply-filters')).toEqual([[[existing, { column: 'data', op, value: expected }]]])
    } finally { wrapper.unmount() }
  })

  it('preserves unfinished drafts and prevents a duplicate condition', async () => {
    const wrapper = mountGrid({ filters: [{ column: 'id', op: '=', value: '1' }] })
    try {
      const filters = wrapper.get('.datagrid__filters')
      filters.findComponent(UiInput).vm.$emit('update:value', '7')
      await wrapper.get('[data-testid="row-inspect"]').trigger('click')
      await flushPromises()
      const picker = wrapper.findAllComponents(UiSelect).find(select => select.attributes('data-testid') === 'grid-inspect-column')!
      picker.vm.$emit('update:value', 'name')
      await flushPromises()
      expect(button()?.disabled).toBe(true)
      expect(filters.findComponent(UiInput).props('value')).toBe('7')
      filters.findComponent(UiInput).vm.$emit('update:value', '1')
      await flushPromises()
      expect(button()?.disabled).toBe(false)
      picker.vm.$emit('update:value', 'id')
      await flushPromises()
      expect(button()?.disabled).toBe(true)
      expect(wrapper.emitted('apply-filters')).toBeUndefined()
    } finally { wrapper.unmount() }
  })

  it.each([
    ['payload', 'bytea', '0x4455'],
    ['nested', 'text[]', '["apple","banana"]'],
    ['big', 'text', 'a'.repeat(5000)],
    ['a:b', 'text', 'unrepresentable column'],
  ])('hides the action for values that cannot safely round-trip in a filter on %s', async (name, type, value) => {
    const wrapper = mountGrid({ columns: [{ name, type, nullable: true }, columns[0]], rows: [[value, 1]] })
    try {
      await wrapper.get('[data-testid="row-inspect"]').trigger('click')
      await flushPromises()
      expect(document.body.querySelector('[data-testid="grid-inspected-value"]')?.textContent).toBe(value)
      expect(button()).toBeNull()
    } finally { wrapper.unmount() }
  })
})

it('keeps compact row actions named and reachable without opening a menu', async () => {
  const wrapper = mountGrid()
  const actions = wrapper.get('tbody tr td:last-child').findAll('button')
  expect(actions.map(button => button.attributes('aria-label'))).toEqual(['Изменить', 'Превью', 'Копировать', 'Удалить'])
  for (const button of actions) expect(button.attributes('title')).toBe(button.attributes('aria-label'))
  const actionColumn = wrapper.get<HTMLTableColElement>('col:last-child')
  expect(parseFloat(actionColumn.element.style.width)).toBeLessThanOrEqual(150)
  await actions[0]!.trigger('click')
  expect(wrapper.emitted('edit-row')?.[0]?.[0]).toMatchObject({id:1,name:'alice'})
  wrapper.unmount()
})

it('dismisses a pending row delete when a keyless position changes', async () => {
  const wrapper = mountGrid({ pkColumns: [] }, document.body)
  const original = wrapper.get('tbody tr').findAllComponents(UiPopconfirm)[0]!
  await original.get('button').trigger('click'); await flushPromises()
  await wrapper.setProps({ rows: [[3, 'new search result'], [2, null]] })
  original.vm.$emit('positive-click'); await flushPromises()
  expect(wrapper.emitted('delete-row')).toBeUndefined()
  wrapper.unmount()
})
