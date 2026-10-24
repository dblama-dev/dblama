import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import {
  NCheckbox,
  NInput,
  NNotificationProvider,
  NRadio,
  NSelect,
} from 'naive-ui'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '../../api/client'
import { useCatalogStore } from '../../stores/catalog'
import type { TableDef } from '../../api/types'
import i18n from '../../i18n'
import StructureEditor from './StructureEditor.vue'

vi.mock('../../api/client', () => ({
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

const mockedApi = vi.mocked(api)

const defUrl = '/api/databases/mydb/tables/users'
const tablesUrl = '/api/databases/mydb/tables'

const def: TableDef = {
  newName: 'users',
  columns: [
    { name: 'id', oldName: 'id', type: 'int', nullable: false, autoIncrement: true },
    {
      name: 'title',
      oldName: 'title',
      type: 'varchar',
      length: '255',
      nullable: true,
      default: { value: 'untitled', isSqlExpression: false },
    },
    {
      name: 'size',
      oldName: 'size',
      type: 'decimal',
      length: '10,2',
      unsigned: true,
      nullable: true,
    },
  ],
  indexes: [{ name: 'PRIMARY', type: 'PRIMARY', columns: [{ name: 'id' }] }],
  foreignKeys: [],
  options: { engine: 'InnoDB', collation: 'utf8mb4_0900_ai_ci', comment: '', autoIncrement: 5 },
}

function mockBase(): void {
  mockedApi.mockImplementation(async (method, url) => {
    if (method === 'GET' && url === defUrl) return def
    if (method === 'GET' && url === tablesUrl) return { tables: [] }
    if (method === 'PUT' && url === defUrl) return undefined
    if (method === 'POST' && url === `${defUrl}/indexes`) return undefined
    throw new Error(`unexpected api call: ${method} ${url}`)
  })
}

function mountEditor(): ReturnType<typeof mount> {
  const pinia = createPinia()
  setActivePinia(pinia)
    // Teleport is stubbed so notification content stays inside the wrapper.
    return mount(
      defineComponent({
        setup() {
          return () =>
            h(NNotificationProvider, null, {
              default: () => h(StructureEditor, { db: 'mydb', table: 'users' }),
            })
        },
      }),
      { global: { plugins: [pinia, i18n], stubs: { teleport: true } } },
    )
}

async function mountLoaded(): Promise<ReturnType<typeof mount>> {
  const wrapper = mountEditor()
  await flushPromises()
  return wrapper
}

function rows(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('[data-testid="struct-row"]')
}

function rowByName(wrapper: ReturnType<typeof mount>, name: string) {
  const row = rows(wrapper).find((r) => nameInput(r).element.value === name)
  expect(row, `row for column ${name}`).toBeDefined()
  return row!
}

function field(row: ReturnType<typeof rows>, cls: string) {
  return row.find(`.${cls}`)
}

function nameInput(row: ReturnType<typeof rows>) {
  const input = field(row, 'struct-field--name').find('input')
  expect(input.exists()).toBe(true)
  return input
}

function setString(row: ReturnType<typeof rows>, cls: string, value: string): void {
  field(row, cls).findComponent(NInput).vm.$emit('update:value', value)
}

function setGenerated(row: ReturnType<typeof rows>, value: string): void {
  field(row, 'struct-field--generated').findComponent(NSelect).vm.$emit('update:value', value)
}

async function clickSave(wrapper: ReturnType<typeof mount>): Promise<void> {
  await wrapper.find('[data-testid="struct-save"]').trigger('click')
  await flushPromises()
}

function putBodies(): TableDef[] {
  return mockedApi.mock.calls
    .filter(([method, url]) => method === 'PUT' && url === defUrl)
    .map((call) => call[2] as TableDef)
}

afterEach(() => {
  vi.clearAllMocks()
})

describe('StructureEditor', () => {
  it('renders one row per column of the loaded definition', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    const names = rows(wrapper).map((r) => nameInput(r).element.value)
    expect(names).toEqual(['id', 'title', 'size'])
    expect(field(rowByName(wrapper, 'id'), 'struct-field--autoinc').findComponent(NRadio).props('checked')).toBe(true)
    expect(field(rowByName(wrapper, 'title'), 'struct-field--default').findComponent(NInput).props('value')).toBe('untitled')
    wrapper.unmount()
  })

  it('renaming a column sends oldName = original name and name = new name', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    setString(rowByName(wrapper, 'title'), 'struct-field--name', 'heading')
    await clickSave(wrapper)

    const body = putBodies()[0]
    const title = body.columns.find((c) => c.name === 'heading')
    expect(title).toMatchObject({ name: 'heading', oldName: 'title' })
    // Unchanged columns keep oldName = name.
    expect(body.columns.find((c) => c.name === 'id')).toMatchObject({ oldName: 'id' })
    wrapper.unmount()
  })

  it('marking a column for deletion sends a row with an empty type', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    const row = rowByName(wrapper, 'size')
    expect(row.classes()).not.toContain('struct-row--deleted')
    await row.find('[data-testid="col-delete"]').trigger('click')
    expect(rowByName(wrapper, 'size').classes()).toContain('struct-row--deleted')

    await clickSave(wrapper)

    const body = putBodies()[0]
    expect(body.columns.find((c) => c.name === 'size')).toEqual({
      name: 'size',
      oldName: 'size',
      type: '',
    })
    wrapper.unmount()
  })

  it('an added column is sent without oldName and positioned after the previous column', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    await wrapper.find('[data-testid="struct-add"]').trigger('click')
    await flushPromises()

    const added = rows(wrapper)[3]
    setString(added, 'struct-field--name', 'extra')
    setGenerated(added, '') // touch the type select below via its own field
    field(added, 'struct-field--type').findComponent(NSelect).vm.$emit('update:value', 'text')
    // Raw $emit does not flush the render queue: wait for the save button's
    // disabled state to clear before clicking it.
    await flushPromises()

    await clickSave(wrapper)

    const extra = putBodies()[0].columns.find((c) => c.name === 'extra')
    expect(extra).toMatchObject({ name: 'extra', type: 'text', position: 'size' })
    expect(extra && 'oldName' in extra).toBe(false)
    wrapper.unmount()
  })

  it('moving a column up swaps its order in the PUT body', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    await rowByName(wrapper, 'title').find('[data-testid="col-up"]').trigger('click')
    await clickSave(wrapper)

    expect(putBodies()[0].columns.map((c) => c.name)).toEqual(['title', 'id', 'size'])
    wrapper.unmount()
  })

  it('auto_increment is exclusive: checking one column unchecks the other', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    const titleRadio = field(rowByName(wrapper, 'title'), 'struct-field--autoinc').findComponent(NRadio)
    titleRadio.vm.$emit('update:checked', true)
    await flushPromises()

    expect(field(rowByName(wrapper, 'title'), 'struct-field--autoinc').findComponent(NRadio).props('checked')).toBe(true)
    expect(field(rowByName(wrapper, 'id'), 'struct-field--autoinc').findComponent(NRadio).props('checked')).toBe(false)

    await clickSave(wrapper)
    const columns = putBodies()[0].columns
    expect(columns.find((c) => c.name === 'title')?.autoIncrement).toBe(true)
    expect(columns.find((c) => c.name === 'id')?.autoIncrement).toBeUndefined()
    wrapper.unmount()
  })

  it('a generated column forces nullable off and carries the expression as an SQL default', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    await wrapper.find('[data-testid="struct-add"]').trigger('click')
    await flushPromises()
    const added = rows(wrapper)[3]
    setString(added, 'struct-field--name', 'dbl')
    field(added, 'struct-field--type').findComponent(NSelect).vm.$emit('update:value', 'int')
    setGenerated(added, 'VIRTUAL')
    // The generated select swaps the default field for the expression
    // field via v-if; flush so the new field is in the DOM.
    await flushPromises()
    setString(added, 'struct-field--genexpr', 'id * 2')
    await flushPromises()

    expect(field(added, 'struct-field--nullable').findComponent(NCheckbox).props('disabled')).toBe(true)

    await clickSave(wrapper)
    const dbl = putBodies()[0].columns.find((c) => c.name === 'dbl')
    expect(dbl).toMatchObject({
      generated: 'VIRTUAL',
      nullable: false,
      default: { value: 'id * 2', isSqlExpression: true },
    })
    wrapper.unmount()
  })

  it('save issues exactly one PUT with columns and options and the indexes as loaded', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    // Options are editable and travel in the same PUT.
    field(
      wrapper.find('.struct-options'),
      'struct-field--engine',
    )
      .findComponent(NInput)
      .vm.$emit('update:value', 'Aria')

    await clickSave(wrapper)

    const puts = putBodies()
    expect(puts).toHaveLength(1)
    expect(puts[0]).toEqual({
      newName: 'users',
      columns: [
        { name: 'id', oldName: 'id', type: 'int', nullable: false, autoIncrement: true },
        {
          name: 'title',
          oldName: 'title',
          type: 'varchar',
          length: '255',
          nullable: true,
          default: { value: 'untitled', isSqlExpression: false },
        },
        {
          name: 'size',
          oldName: 'size',
          type: 'decimal',
          length: '10,2',
          unsigned: true,
          nullable: true,
        },
      ],
      indexes: def.indexes,
      foreignKeys: [],
      options: { engine: 'Aria', collation: 'utf8mb4_0900_ai_ci', comment: '', autoIncrement: 5 },
    })
    wrapper.unmount()
  })

  it('after a successful save the catalog is invalidated and the definition is refetched', async () => {
    mockBase()
    const wrapper = await mountLoaded()
    const catalog = useCatalogStore()
    const invalidate = vi.spyOn(catalog, 'invalidate')
    const defGets = () => mockedApi.mock.calls.filter(([m, u]) => m === 'GET' && u === defUrl).length

    const before = defGets()
    await clickSave(wrapper)

    expect(invalidate).toHaveBeenCalledWith('mydb')
    expect(defGets()).toBeGreaterThan(before)
    wrapper.unmount()
  })

  it('a failed save shows the error message and does not invalidate', async () => {
    const { ApiError } = await import('../../api/client')
    mockedApi.mockImplementation(async (method, url) => {
      if (method === 'GET' && url === defUrl) return def
      if (method === 'PUT' && url === defUrl) {
        throw new ApiError('bad_request', 'step 2: invalid type', 400)
      }
      throw new Error(`unexpected api call: ${method} ${url}`)
    })
    const wrapper = await mountLoaded()
    const catalog = useCatalogStore()
    const invalidate = vi.spyOn(catalog, 'invalidate')

    await clickSave(wrapper)

    expect(wrapper.text()).toContain('step 2: invalid type')
    expect(invalidate).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('saving indexes posts the full desired list to the indexes endpoint', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    await wrapper.find('[data-testid="index-save"]').trigger('click')
    await flushPromises()

    expect(mockedApi).toHaveBeenCalledWith('POST', `${defUrl}/indexes`, [
      { name: 'PRIMARY', type: 'PRIMARY', columns: [{ name: 'id' }] },
    ])
    wrapper.unmount()
  })
})
