import { flushPromises, mount, type DOMWrapper, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h, reactive } from 'vue'
import {
  NCheckbox,
  NInput,
  NNotificationProvider,
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

function mountEditor(table = 'users', schema?: string): ReturnType<typeof mount> {
  const pinia = createPinia()
  setActivePinia(pinia)
  if (schema) { const c = useCatalogStore(); c.schemaAware = true; c.schema = schema }
    // Teleport is stubbed so notification content stays inside the wrapper.
    return mount(
      defineComponent({
        setup() {
          return () =>
            h(NNotificationProvider, null, {
              default: () => h(StructureEditor, { db: 'mydb', table }),
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

function field(row: DOMWrapper<Element> | VueWrapper, cls: string) {
  return row.find(`.${cls}`)
}

function nameInput(row: DOMWrapper<Element> | VueWrapper) {
  const input = field(row, 'struct-field--name').find('input')
  expect(input.exists()).toBe(true)
  return input
}

function setString(row: DOMWrapper<Element> | VueWrapper, cls: string, value: string): void {
  field(row, cls).findComponent(NInput).vm.$emit('update:value', value)
}

function setGenerated(row: DOMWrapper<Element> | VueWrapper, value: string): void {
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
    expect(field(rowByName(wrapper, 'id'), 'struct-field--autoinc').findComponent(NCheckbox).props('checked')).toBe(true)
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
      nullable: false,
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
    // Every row carries its placement from the new display order — that
    // is what makes the reorder a server-side effect (MODIFY ... FIRST /
    // AFTER) instead of a silent no-op.
    const positions = Object.fromEntries(
      putBodies()[0].columns.map((c) => [c.name, c.position]),
    )
    expect(positions).toEqual({ title: 'FIRST', id: 'title', size: 'id' })
    wrapper.unmount()
  })

  it('turns off a loaded auto_increment column with a real click', async () => {
    mockBase()
    const wrapper = await mountLoaded()
    const toggle = field(rowByName(wrapper, 'id'), 'struct-field--autoinc').find('.n-radio, .n-checkbox')
    await toggle.trigger('click')
    await clickSave(wrapper)
    expect(putBodies()[0].columns.find(c => c.name === 'id')?.autoIncrement).toBeUndefined()
    wrapper.unmount()
  })

  it('disabling a PostgreSQL serial also removes its nextval default', async () => {
    mockedApi.mockImplementation(async (method, url) => {
      if (method === 'GET' && url.startsWith(defUrl)) return {
        ...def, columns: [{ name: 'id', oldName: 'id', type: 'int', nullable: false, autoIncrement: true,
          default: { value: "nextval('users_id_seq'::regclass)", isSqlExpression: true } }],
      }
      return undefined
    })
    const wrapper = mountEditor('users', 'public')
    await flushPromises()
    await field(rowByName(wrapper, 'id'), 'struct-field--autoinc').find('.n-checkbox').trigger('click')
    await clickSave(wrapper)
    const body = mockedApi.mock.calls.find(([method]) => method === 'PUT')![2] as TableDef
    expect(body.columns[0].autoIncrement).toBeUndefined()
    expect(body.columns[0].default).toBeUndefined()
    wrapper.unmount()
  })

  it('auto_increment is exclusive: checking one column unchecks the other', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    const titleCheckbox = field(rowByName(wrapper, 'title'), 'struct-field--autoinc').findComponent(NCheckbox)
    titleCheckbox.vm.$emit('update:checked', true)
    await flushPromises()

    expect(field(rowByName(wrapper, 'title'), 'struct-field--autoinc').findComponent(NCheckbox).props('checked')).toBe(true)
    expect(field(rowByName(wrapper, 'id'), 'struct-field--autoinc').findComponent(NCheckbox).props('checked')).toBe(false)

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
        // Every column carries its position from the display order (the
        // backend renders FIRST/AFTER for MODIFY as well; an unchanged
        // position is an idempotent no-op on the server).
        { name: 'id', oldName: 'id', type: 'int', nullable: false, autoIncrement: true, position: 'FIRST' },
        {
          name: 'title',
          oldName: 'title',
          type: 'varchar',
          length: '255',
          nullable: true,
          default: { value: 'untitled', isSqlExpression: false },
          position: 'id',
        },
        {
          name: 'size',
          oldName: 'size',
          type: 'decimal',
          length: '10,2',
          unsigned: true,
          nullable: true,
          position: 'title',
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

  it('create mode: no def fetch, PUT to tables/_new and a created event with the new name', async () => {
    const newUrl = '/api/databases/mydb/tables/_new'
    mockedApi.mockImplementation(async (method, url) => {
      if (method === 'PUT' && url === newUrl) return undefined
      throw new Error(`unexpected api call: ${method} ${url}`)
    })
    const wrapper = mountEditor('_new')
    await flushPromises()

    // The _new placeholder has no definition behind it: no GET.
    expect(mockedApi).not.toHaveBeenCalledWith('GET', newUrl)
    // The table name field starts empty and the save button is disabled
    // until a table name and at least one column exist.
    expect(wrapper.find('[data-testid="struct-save"]').attributes('disabled')).toBeDefined()

    wrapper.find('.struct-field--table-name').findComponent(NInput).vm.$emit('update:value', 'newtbl')
    await wrapper.find('[data-testid="struct-add"]').trigger('click')
    await flushPromises()
    const added = rows(wrapper)[0]
    setString(added, 'struct-field--name', 'id')
    field(added, 'struct-field--type').findComponent(NSelect).vm.$emit('update:value', 'int')
    await flushPromises()

    expect(wrapper.find('[data-testid="struct-save"]').attributes('disabled')).toBeUndefined()
    await clickSave(wrapper)

    expect(mockedApi).toHaveBeenCalledWith('PUT', newUrl, {
      newName: 'newtbl',
      columns: [{ name: 'id', type: 'int', nullable: true, position: 'FIRST' }],
      indexes: [],
      foreignKeys: [],
      options: {},
    })
    expect(wrapper.findComponent(StructureEditor).emitted('created')).toEqual([['newtbl']])
    wrapper.unmount()
  })

  it('includes a draft primary key in the same request that creates the table', async () => {
    mockedApi.mockResolvedValue(undefined)
    const wrapper = mountEditor('_new')
    await flushPromises()
    setString(wrapper, 'struct-field--table-name', 'items')
    await wrapper.find('[data-testid="struct-add"]').trigger('click')
    const row = rows(wrapper)[0]
    setString(row, 'struct-field--name', 'id')
    field(row, 'struct-field--type').findComponent(NSelect).vm.$emit('update:value', 'int')
    await flushPromises()
    const addIndex = wrapper.findAll('button').find(button => button.text() === i18n.global.t('structure.addIndex'))
    expect(addIndex, 'new tables must expose index editing').toBeDefined()
    await addIndex!.trigger('click')
    expect(wrapper.find('[data-testid="struct-save"]').attributes('disabled')).toBeDefined()
    const index = wrapper.find('[data-testid="index-row"]')
    index.find('.struct-field--indextype').findComponent(NSelect).vm.$emit('update:value', 'PRIMARY')
    index.find('.struct-field--indexcolumns').findComponent(NSelect).vm.$emit('update:value', ['id'])
    await flushPromises()
    await clickSave(wrapper)
    expect(mockedApi).toHaveBeenCalledWith('PUT', '/api/databases/mydb/tables/_new', expect.objectContaining({
      indexes: [{ name: 'PRIMARY', type: 'PRIMARY', columns: [{ name: 'id' }] }],
    }))
    expect(mockedApi.mock.calls.some(([method]) => method === 'POST')).toBe(false)
    wrapper.unmount()
  })

  it('resets the editor when navigating from a table to create mode', async () => {
    mockBase()
    // vue-router reuses the component instance when only the table param
    // changes (users → _new), so the props must be reactive here.
    const state = reactive({ db: 'mydb', table: 'users' })
    const pinia = createPinia()
    setActivePinia(pinia)
    const wrapper = mount(
      defineComponent({
        setup() {
          return () =>
            h(NNotificationProvider, null, {
              default: () => h(StructureEditor, { db: state.db, table: state.table }),
            })
        },
      }),
      { global: { plugins: [pinia, i18n], stubs: { teleport: true } } },
    )
    await flushPromises()
    expect(rows(wrapper)).toHaveLength(3)
    expect(wrapper.find('.struct-field--table-name').find('input').element.value).toBe('users')

    state.table = '_new'
    await flushPromises()

    // Create mode owns no definition: the editor must not show the
    // previous table's structure (and no def GET fires for _new —
    // mockBase would throw on one).
    expect(rows(wrapper)).toHaveLength(0)
    expect(wrapper.find('.struct-field--table-name').find('input').element.value).toBe('')
    wrapper.unmount()
  })
})

it('uses schema for definitions and writes and hides PostgreSQL column reordering', async () => {
  mockedApi.mockImplementation(async (method, url) => {
    if (method === 'GET' && url.includes('/tables?')) return { tables: [] }
    if (method === 'GET') return def
    return undefined
  })
  const wrapper = mountEditor('users', 'Odd Schema')
  await flushPromises()
  expect(mockedApi).toHaveBeenCalledWith('GET', `${defUrl}?schema=Odd%20Schema`)
  expect(wrapper.find('[data-testid="col-up"]').exists()).toBe(false)
  await wrapper.find('[data-testid="struct-save"]').trigger('click')
  await flushPromises()
  expect(mockedApi).toHaveBeenCalledWith('PUT', `${defUrl}?schema=Odd%20Schema`, expect.anything())
  wrapper.unmount()
})
