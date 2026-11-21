import { flushPromises, mount, type DOMWrapper, type VueWrapper } from '@vue/test-utils'
import { defineComponent, h, reactive } from 'vue'
import {
  UiCheckbox,
  UiInput,
  UiNotificationProvider,
  UiSelect,
} from '../../ui'
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
            h(UiNotificationProvider, null, {
              default: () => h(StructureEditor, { db: 'mydb', table }),
            })
        },
      }),
      { attachTo: document.body, global: { plugins: [pinia, i18n], stubs: { teleport: true } } },
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
  field(row, cls).findComponent(UiInput).vm.$emit('update:value', value)
}

function setGenerated(row: DOMWrapper<Element> | VueWrapper, value: string): void {
  field(row, 'struct-field--generated').findComponent(UiSelect).vm.$emit('update:value', value)
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
  it('offers table engines, index prefixes and MySQL index types only on compatible drivers', async () => {
    mockBase()
    const wrapper = await mountLoaded()
    expect(wrapper.find('.struct-field--engine').exists()).toBe(true)
    expect(wrapper.find('.index-editor__subpart').exists()).toBe(true)
    const indexTypes = () => wrapper.get('.struct-field--indextype').findAll('option').map(option => option.text())
    expect(indexTypes()).toContain('FULLTEXT')
    useCatalogStore().schemaAware = true
    await flushPromises()
    expect(wrapper.find('.struct-field--engine').exists()).toBe(false)
    expect(wrapper.find('.struct-options .struct-field--collation').exists()).toBe(false)
    expect(wrapper.find('.struct-options .struct-field--autoinc').exists()).toBe(false)
    expect(wrapper.find('.struct-options .struct-field--comment').exists()).toBe(true)
    expect(wrapper.find('.index-editor__subpart').exists()).toBe(false)
    expect(wrapper.find('.struct-field--unsigned').exists()).toBe(false)
    expect(wrapper.find('.struct-field--onupdate').exists()).toBe(false)
    expect(wrapper.find('.struct-field--autoinc').text()).toBe('Автонумерация')
    expect(indexTypes()).toEqual(['PRIMARY', 'UNIQUE', 'INDEX'])
    wrapper.unmount()
  })

  it('renders one row per column of the loaded definition', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    const names = rows(wrapper).map((r) => nameInput(r).element.value)
    expect(names).toEqual(['id', 'title', 'size'])
    expect(field(rowByName(wrapper, 'id'), 'struct-field--autoinc').findComponent(UiCheckbox).props('checked')).toBe(true)
    expect(field(rowByName(wrapper, 'title'), 'struct-field--default').findComponent(UiInput).props('value')).toBe('untitled')
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
    field(added, 'struct-field--type').findComponent(UiSelect).vm.$emit('update:value', 'text')
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

  it('inserts columns at the start and after a renamed anchor directly from the overview', async () => {
    mockBase()
    const wrapper = await mountLoaded()
    setString(rowByName(wrapper, 'title'), 'struct-field--name', 'FiRsT')
    await wrapper.get('[data-add-first]').trigger('click')
    const leading = rows(wrapper)[0]
    setString(leading, 'struct-field--name', 'leading')
    field(leading, 'struct-field--type').findComponent(UiSelect).vm.$emit('update:value', 'int')
    const anchor = rowByName(wrapper, 'FiRsT')
    const insert = anchor.element.parentElement!.querySelector<HTMLButtonElement>('[data-add-after]')!
    expect(insert).toBeTruthy()
    insert.click()
    await flushPromises()
    const inserted = rows(wrapper)[3]
    expect(inserted.attributes('open')).toBeDefined()
    setString(inserted, 'struct-field--name', 'inserted')
    field(inserted, 'struct-field--type').findComponent(UiSelect).vm.$emit('update:value', 'text')
    await flushPromises()
    await clickSave(wrapper)
    expect(putBodies()[0].columns.map(c => [c.name, c.position, c.afterColumn])).toEqual([
      ['leading', 'FIRST', undefined], ['id', 'leading', undefined], ['FiRsT', 'id', undefined],
      ['inserted', undefined, 'FiRsT'], ['size', 'inserted', undefined],
    ])
    wrapper.unmount()
  })

  it('allows choosing column order before creating a PostgreSQL table and omits placement clauses', async () => {
    mockedApi.mockResolvedValue(undefined)
    const wrapper = mountEditor('_new', 'public')
    await flushPromises()
    setString(wrapper, 'struct-field--table-name', 'events')
    await wrapper.get('[data-testid="struct-add"]').trigger('click')
    setString(rows(wrapper)[0], 'struct-field--name', 'value')
    field(rows(wrapper)[0], 'struct-field--type').findComponent(UiSelect).vm.$emit('update:value', 'text')
    await wrapper.get('[data-add-first]').trigger('click')
    setString(rows(wrapper)[0], 'struct-field--name', 'id')
    field(rows(wrapper)[0], 'struct-field--type').findComponent(UiSelect).vm.$emit('update:value', 'int')
    await flushPromises()
    await clickSave(wrapper)
    const body = mockedApi.mock.calls.find(([method]) => method === 'PUT')![2] as TableDef
    expect(body.columns).toEqual([{ name: 'id', type: 'int', nullable: true }, { name: 'value', type: 'text', nullable: true }])
    wrapper.unmount()
  })

  it('turns off a loaded auto_increment column with a real click', async () => {
    mockBase()
    const wrapper = await mountLoaded()
    const toggle = field(rowByName(wrapper, 'id'), 'struct-field--autoinc').get('input[type=checkbox]')
    ;(toggle.element as HTMLInputElement).click()
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
    ;(field(rowByName(wrapper, 'id'), 'struct-field--autoinc').get('input[type=checkbox]').element as HTMLInputElement).click()
    await clickSave(wrapper)
    const body = mockedApi.mock.calls.find(([method]) => method === 'PUT')![2] as TableDef
    expect(body.columns[0].autoIncrement).toBeUndefined()
    expect(body.columns[0].default).toBeUndefined()
    wrapper.unmount()
  })

  it('auto_increment is exclusive: checking one column unchecks the other', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    const titleCheckbox = field(rowByName(wrapper, 'title'), 'struct-field--autoinc').findComponent(UiCheckbox)
    titleCheckbox.vm.$emit('update:checked', true)
    await flushPromises()

    expect(field(rowByName(wrapper, 'title'), 'struct-field--autoinc').findComponent(UiCheckbox).props('checked')).toBe(true)
    expect(field(rowByName(wrapper, 'id'), 'struct-field--autoinc').findComponent(UiCheckbox).props('checked')).toBe(false)

    await clickSave(wrapper)
    const columns = putBodies()[0].columns
    expect(columns.find((c) => c.name === 'title')?.autoIncrement).toBe(true)
    expect(columns.find((c) => c.name === 'id')?.autoIncrement).toBeUndefined()
    wrapper.unmount()
  })

  it('sends a generated column with the nullable flag the MySQL driver requires', async () => {
    mockBase()
    const wrapper = await mountLoaded()

    await wrapper.find('[data-testid="struct-add"]').trigger('click')
    await flushPromises()
    const added = rows(wrapper)[3]
    setString(added, 'struct-field--name', 'dbl')
    field(added, 'struct-field--type').findComponent(UiSelect).vm.$emit('update:value', 'int')
    setGenerated(added, 'VIRTUAL')
    // The generated select swaps the default field for the expression
    // field via v-if; flush so the new field is in the DOM.
    await flushPromises()
    setString(added, 'struct-field--genexpr', 'id * 2')
    await flushPromises()

    expect(field(added, 'struct-field--nullable').findComponent(UiCheckbox).props('disabled')).toBe(true)

    await clickSave(wrapper)
    const dbl = putBodies()[0].columns.find((c) => c.name === 'dbl')
    expect(dbl).toMatchObject({
      generated: 'VIRTUAL',
      nullable: true,
      default: { value: 'id * 2', isSqlExpression: true },
    })
    wrapper.unmount()
  })

  it.each(['postgres', 'mysql'])('offers generation controls supported for existing and new %s columns', async engine => {
    const postgres = engine === 'postgres'
    mockedApi.mockImplementation(async (method, url) => {
      if (method === 'GET' && url.startsWith(defUrl)) return {
        ...def, columns: [...def.columns, {
          name: 'computed', type: 'varchar', length: '20', nullable: true, generated: 'STORED', default: { value: 'upper(title)', isSqlExpression: true },
        }],
      }
      return undefined
    })
    const wrapper = mountEditor('users', postgres ? 'public' : undefined)
    await flushPromises()
    const computed = rowByName(wrapper, 'computed')
    expect(field(computed, 'struct-field--generated').findComponent(UiSelect).props('disabled')).toBe(postgres)
    expect(field(computed, 'struct-field--type').findComponent(UiSelect).props('disabled')).toBe(postgres)
    expect(field(computed, 'struct-field--length').findComponent(UiInput).props('readonly')).toBe(postgres)
    expect(field(computed, 'struct-field--genexpr').findComponent(UiInput).props('readonly')).toBe(postgres)
    expect(field(computed, 'struct-field--nullable').findComponent(UiCheckbox).props('disabled')).toBe(!postgres)
    expect(field(computed, 'struct-field--name').findComponent(UiInput).props('disabled')).toBe(false)
    expect(field(computed, 'struct-field--generated').text().includes(i18n.global.t('structure.savedGenerationHint'))).toBe(postgres)
    const ordinary = rowByName(wrapper, 'title')
    expect(field(ordinary, 'struct-field--generated').findComponent(UiSelect).props('disabled')).toBe(postgres)
    expect(field(ordinary, 'struct-field--type').findComponent(UiSelect).props('disabled')).toBe(false)
    await wrapper.get('[data-testid="struct-add"]').trigger('click')
    const added = rows(wrapper).at(-1)!
    expect(field(added, 'struct-field--generated').findComponent(UiSelect).props('disabled')).toBe(false)
    setGenerated(added, 'STORED')
    await flushPromises()
    expect(field(added, 'struct-field--genexpr').findComponent(UiInput).props('readonly')).toBe(false)
    wrapper.unmount()
  })

  it('preserves an empty literal default and each ENUM label when saving another column', async () => {
    const choices = ['', 'red, green', "O'Brien", ' leading space']
    mockedApi.mockImplementation(async (method, url) => {
      if (method === 'GET' && url === defUrl) return { ...def, columns: [
        ...def.columns,
        { name: 'choice', type: 'enum', nullable: false, enumValues: choices, default: { value: '' } },
      ] }
      return undefined
    })
    const wrapper = await mountLoaded()
    setString(rowByName(wrapper, 'title'), 'struct-field--name', 'heading')
    await clickSave(wrapper)
    expect(putBodies()[0].columns.find(c => c.name === 'choice')).toMatchObject({
      enumValues: choices, default: { value: '', isSqlExpression: false },
    })
    expect(rowByName(wrapper, 'choice').get('summary').text()).toContain("DEFAULT ''")
    wrapper.unmount()
  })

  it('can create an empty-string default, remove a default, set NULL and enter an expression', async () => {
    mockBase()
    const wrapper = await mountLoaded()
    async function setMode(name: string, mode: string) {
      await rowByName(wrapper, name).get('.default-mode select').setValue(mode)
    }
    await setMode('id', 'none')
    await setMode('title', 'literal')
    setString(rowByName(wrapper, 'title'), 'struct-field--default', '')
    await setMode('size', 'null')
    await clickSave(wrapper)
    expect(putBodies()[0].columns.map(c => c.default)).toEqual([
      undefined, { value: '', isSqlExpression: false }, { value: 'NULL', isSqlExpression: true },
    ])
    await setMode('title', 'none')
    await setMode('size', 'expression')
    expect(wrapper.get('[data-testid="struct-save"]').attributes('disabled')).toBeDefined()
    setString(rowByName(wrapper, 'size'), 'struct-field--default', '(1 + 2)')
    await flushPromises()
    await clickSave(wrapper)
    expect(putBodies()[1].columns.map(c => c.default)).toEqual([
      undefined, undefined, { value: '(1 + 2)', isSqlExpression: true },
    ])
    wrapper.unmount()
  })

  it.each(['enum', 'set'])('edits ordered %s choices individually', async type => {
    mockBase()
    const wrapper = await mountLoaded()
    const row = rowByName(wrapper, 'title')
    field(row, 'struct-field--type').findComponent(UiSelect).vm.$emit('update:value', type)
    await flushPromises()
    expect(wrapper.get('[data-testid="struct-save"]').attributes('disabled')).toBeDefined()
    for (const value of ['remove me', "won't trim", '']) {
      await row.get('[data-add-choice]').trigger('click')
      await row.findAll('textarea.enum-choice').at(-1)!.setValue(value)
    }
    await row.get('[data-remove-choice]').trigger('click')
    await clickSave(wrapper)
    expect(putBodies()[0].columns.find(c => c.name === 'title')?.enumValues).toEqual(["won't trim", ''])
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
      .findComponent(UiInput)
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

    wrapper.find('.struct-field--table-name').findComponent(UiInput).vm.$emit('update:value', 'newtbl')
    await wrapper.find('[data-testid="struct-add"]').trigger('click')
    await flushPromises()
    const added = rows(wrapper)[0]
    setString(added, 'struct-field--name', 'id')
    field(added, 'struct-field--type').findComponent(UiSelect).vm.$emit('update:value', 'int')
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
    field(row, 'struct-field--type').findComponent(UiSelect).vm.$emit('update:value', 'int')
    await flushPromises()
    const addIndex = wrapper.findAll('button').find(button => button.text() === i18n.global.t('structure.addIndex'))
    expect(addIndex, 'new tables must expose index editing').toBeDefined()
    await addIndex!.trigger('click')
    expect(wrapper.find('[data-testid="struct-save"]').attributes('disabled')).toBeDefined()
    const index = wrapper.find('[data-testid="index-row"]')
    index.find('.struct-field--indextype').findComponent(UiSelect).vm.$emit('update:value', 'PRIMARY')
    index.find('.struct-field--indexcolumns').findComponent(UiSelect).vm.$emit('update:value', ['id'])
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
            h(UiNotificationProvider, null, {
              default: () => h(StructureEditor, { db: state.db, table: state.table }),
            })
        },
      }),
      { attachTo: document.body, global: { plugins: [pinia, i18n], stubs: { teleport: true } } },
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
  expect(wrapper.find('[data-add-after]').exists()).toBe(false)
  expect(wrapper.find('[data-add-first]').exists()).toBe(false)
  await wrapper.find('[data-testid="struct-save"]').trigger('click')
  await flushPromises()
  expect(mockedApi).toHaveBeenCalledWith('PUT', `${defUrl}?schema=Odd%20Schema`, expect.anything())
  wrapper.unmount()
})

it('keeps pending columns and FK edits when indexes are saved',async()=>{
 mockBase();const wrapper=await mountLoaded()
 const row=rows(wrapper)[1]!
 row.findAllComponents(UiInput)[0]!.vm.$emit('update:value','pending_title');await flushPromises()
 const {default:IndexEditor}=await import('./IndexEditor.vue')
 wrapper.findComponent(IndexEditor).vm.$emit('saved');await flushPromises()
 expect(rows(wrapper)[1]!.findAllComponents(UiInput)[0]!.props('value')).toBe('pending_title')
 wrapper.unmount()
})
it('starts existing column editors collapsed with readable overview',async()=>{
 mockBase();const wrapper=await mountLoaded()
 expect(wrapper.findAll('details.struct-row')).toHaveLength(3)
 expect(wrapper.find('details.struct-row').attributes('open')).toBeUndefined()
 expect(wrapper.find('details.struct-row summary').text()).toContain('id')
 wrapper.unmount()
})

it('does not warn after load, and allows cancelling loss of any dirty section',async()=>{
 mockBase()
 const {routerKey}=await import('vue-router')
 let guard!: (to:{fullPath:string},from:{fullPath:string})=>boolean
 const pinia=createPinia();setActivePinia(pinia)
 const remove=vi.fn()
 const wrapper=mount(defineComponent({setup:()=>()=>h(UiNotificationProvider,null,{default:()=>h(StructureEditor,{db:'mydb',table:'users'})})}),{global:{plugins:[pinia,i18n],provide:{[routerKey as symbol]:{beforeEach:(fn:typeof guard)=>{guard=fn;return remove}}}}})
 await flushPromises()
 const confirm=vi.spyOn(window,'confirm').mockReturnValue(false)
 expect(guard({fullPath:'/other'},{fullPath:'/structure'})).toBe(true)
 const {default:IndexEditor}=await import('./IndexEditor.vue')
 wrapper.findComponent(IndexEditor).vm.$emit('update:indexes',[{name:'pending',type:'INDEX',columns:[{name:'id'}]}]);await flushPromises()
 expect(guard({fullPath:'/other'},{fullPath:'/structure'})).toBe(false)
 expect(confirm).toHaveBeenCalledOnce()
 wrapper.unmount();expect(remove).toHaveBeenCalledOnce();confirm.mockRestore()
})
it('keeps unsaved index and FK models after saving columns/options',async()=>{
 mockBase();const wrapper=await mountLoaded()
 const {default:IndexEditor}=await import('./IndexEditor.vue')
 const {default:ForeignKeyEditor}=await import('./ForeignKeyEditor.vue')
 const pendingIndex=[{name:'pending',type:'INDEX',columns:[{name:'id'}]}]
 const pendingFK=[{name:'pending_fk',columns:['id'],refTable:'other',refColumns:['id']}]
 wrapper.findComponent(IndexEditor).vm.$emit('update:indexes',pendingIndex)
 wrapper.findComponent(ForeignKeyEditor).vm.$emit('update:modelValue',pendingFK)
 await flushPromises();await clickSave(wrapper)
 expect(wrapper.findComponent(IndexEditor).props('indexes')).toEqual(pendingIndex)
 expect(wrapper.findComponent(ForeignKeyEditor).props('modelValue')).toEqual(pendingFK)
 wrapper.unmount()
})
it('refreshes clean indexes and foreign keys after a saved column rename',async()=>{
 let current=def
 mockedApi.mockImplementation(async(method,url)=>{
  if(method==='GET'&&url===defUrl)return current
  if(method==='GET'&&url===tablesUrl)return {tables:[]}
  if(method==='PUT'){
   current={...def,columns:def.columns.map(column=>column.name==='id'?{...column,name:'renamed_id',oldName:'renamed_id'}:column),indexes:[{name:'PRIMARY',type:'PRIMARY',columns:[{name:'renamed_id'}]}],foreignKeys:[{name:'fk_renamed',columns:['renamed_id'],refTable:'parent',refColumns:['id']}]}
   return
  }
 })
 const wrapper=await mountLoaded()
 setString(rowByName(wrapper,'id'),'struct-field--name','renamed_id')
 await clickSave(wrapper)
 const {default:IndexEditor}=await import('./IndexEditor.vue')
 const {default:ForeignKeyEditor}=await import('./ForeignKeyEditor.vue')
 expect(wrapper.findComponent(IndexEditor).props('indexes')).toEqual(current.indexes)
 expect(wrapper.findComponent(ForeignKeyEditor).props('modelValue')).toEqual(current.foreignKeys)
 const unload=new Event('beforeunload',{cancelable:true});window.dispatchEvent(unload)
 expect(unload.defaultPrevented).toBe(false)
 wrapper.unmount()
})
it('keeps an index edit dirty when it was made during an in-flight save',async()=>{
 let finish!:()=>void
 mockedApi.mockImplementation(async(method,url)=>{
  if(method==='GET'&&url===defUrl)return {...def,indexes:[{name:'original',type:'INDEX',columns:[{name:'id'}]}]}
  if(method==='GET'&&url===tablesUrl)return {tables:[]}
  if(method==='POST'&&url===`${defUrl}/indexes`)return new Promise<void>(resolve=>{finish=resolve})
 })
 const {routerKey}=await import('vue-router')
 let guard!: (to:{fullPath:string},from:{fullPath:string})=>boolean
 const pinia=createPinia();setActivePinia(pinia)
 const wrapper=mount(defineComponent({setup:()=>()=>h(UiNotificationProvider,null,{default:()=>h(StructureEditor,{db:'mydb',table:'users'})})}),{global:{plugins:[pinia,i18n],provide:{[routerKey as symbol]:{beforeEach:(fn:typeof guard)=>{guard=fn;return ()=>{}}}}}})
 await flushPromises()
 const {default:IndexEditor}=await import('./IndexEditor.vue')
 const editor=wrapper.findComponent(IndexEditor)
 const indexName=editor.findComponent(UiInput)
 indexName.vm.$emit('update:value','submitted_A');await flushPromises()
 await editor.get('[data-testid=index-save]').trigger('click')
 indexName.vm.$emit('update:value','unsaved_B');await flushPromises()
 finish();await flushPromises()
 const confirm=vi.spyOn(window,'confirm').mockReturnValue(false)
 expect(guard({fullPath:'/other'},{fullPath:'/structure'})).toBe(false)
 expect(confirm).toHaveBeenCalledOnce()
 expect(indexName.props('value')).toBe('unsaved_B')
 wrapper.unmount();confirm.mockRestore()
})
