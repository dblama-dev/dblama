import { createPinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h } from 'vue'
import {
  NButton,
  NCheckbox,
  NInput,
  NInputNumber,
  NNotificationProvider,
  NSelect,
} from 'naive-ui'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '../api/client'
import type { ColumnDef, Page } from '../api/types'
import i18n from '../i18n'
import RowModal from './RowModal.vue'
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

const rowsPath = '/api/databases/mydb/tables/users'

const columns: ColumnDef[] = [
  { name: 'name', type: 'varchar', nullable: true },
  { name: 'age', type: 'int', nullable: true },
  { name: 'role', type: 'enum', nullable: true, enumValues: ['admin', 'user'] },
  { name: 'bio', type: 'text', nullable: true },
  { name: 'email', type: 'varchar', nullable: false },
  { name: 'id', type: 'int', nullable: false, autoIncrement: true },
]

const ModalInProvider = defineComponent({
  props: {
    show: { type: Boolean, default: true },
    mode: { type: String, default: 'insert' },
    columns: { type: Array, default: () => columns },
    row: { type: Object, default: null },
    rowPk: { type: Object, default: null },
  },
  setup(p) {
    return () =>
      h(NNotificationProvider, null, {
        default: () =>
          h(RowModal, {
            show: p.show,
            mode: p.mode,
            columns: p.columns,
            row: p.row,
            rowPk: p.rowPk,
            rowsPath,
            'onUpdate:show': () => p.show,
          }),
      })
  },
})

// Teleport is stubbed so the modal content stays inside the wrapper and
// the field wrappers can be located per column.
function mountModal(props: Record<string, unknown> = {}) {
  return mount(ModalInProvider, {
    props,
    global: { plugins: [createPinia(), i18n], stubs: { teleport: true } },
  })
}

// Per-field helpers keyed by the column label. Naive-ui's NInputNumber and
// NSelect embed their own NInput internally, so fields are addressed through
// their wrapper element, not by a global NInput index.
function fieldOf(wrapper: ReturnType<typeof mount>, name: string) {
  const field = wrapper
    .findAll('.row-modal__field')
    .find((f) => f.find('.row-modal__label').text() === name)
  expect(field, `field for column ${name}`).toBeDefined()
  return field!
}

function setString(wrapper: ReturnType<typeof mount>, name: string, value: string): void {
  fieldOf(wrapper, name).findComponent(NInput).vm.$emit('update:value', value)
}

function setNumber(wrapper: ReturnType<typeof mount>, name: string, value: number | null): void {
  fieldOf(wrapper, name).findComponent(NInputNumber).vm.$emit('update:value', value)
}

function checkNull(wrapper: ReturnType<typeof mount>, name: string): void {
  fieldOf(wrapper, name).findComponent(NCheckbox).vm.$emit('update:checked', true)
}

function saveButton(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAllComponents(NButton).find((b) => b.text() === 'Сохранить')
}

describe('RowModal', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders one typed field per column: number, select, textarea, text', () => {
    const wrapper = mountModal()

    const names = wrapper.findAll('.row-modal__field').map((f) => f.find('.row-modal__label').text())
    expect(names).toEqual(['name', 'age', 'role', 'bio', 'email', 'id'])

    expect(fieldOf(wrapper, 'name').findComponent(NInput).props('type')).toBe('text')
    expect(fieldOf(wrapper, 'bio').findComponent(NInput).props('type')).toBe('textarea')

    expect(fieldOf(wrapper, 'age').findComponent(NInputNumber).exists()).toBe(true)
    expect(fieldOf(wrapper, 'id').findComponent(NInputNumber).exists()).toBe(true)

    const roleSelect = fieldOf(wrapper, 'role').findComponent(NSelect)
    expect(roleSelect.exists()).toBe(true)
    expect(roleSelect.props('options')).toEqual([
      { label: 'admin', value: 'admin' },
      { label: 'user', value: 'user' },
    ])
    expect(fieldOf(wrapper, 'role').findComponent(NInputNumber).exists()).toBe(false)
  })

  it('shows a NULL toggle only for nullable columns', () => {
    const wrapper = mountModal()

    // 4 nullable columns get a toggle; email and id do not.
    expect(wrapper.findAllComponents(NCheckbox)).toHaveLength(4)
    expect(fieldOf(wrapper, 'email').findComponent(NCheckbox).exists()).toBe(false)
    expect(fieldOf(wrapper, 'id').findComponent(NCheckbox).exists()).toBe(false)
    expect(wrapper.text()).toContain('обязательно')
  })

  it('disables the input and sends null when the NULL toggle is checked', async () => {
    mockedApi.mockResolvedValue({ lastInsertId: 1 })
    const wrapper = mountModal()

    setString(wrapper, 'name', 'bob')
    await flushPromises()
    checkNull(wrapper, 'name')
    await flushPromises()

    expect(fieldOf(wrapper, 'name').findComponent(NInput).props('disabled')).toBe(true)

    // The non-nullable required fields must be filled for Save to enable.
    setString(wrapper, 'email', 'x@y.z')
    setNumber(wrapper, 'id', 7)
    await flushPromises()

    await saveButton(wrapper)?.trigger('click')
    await flushPromises()

    const modal = wrapper.findComponent(RowModal)
    expect(mockedApi).toHaveBeenCalledWith('POST', rowsPath, expect.objectContaining({ name: null }))
    expect(modal.emitted('saved')).toHaveLength(1)
  })

  it('posts the full RowValues on insert: null if toggled, number or null for numerics, raw strings', async () => {
    mockedApi.mockResolvedValue({ lastInsertId: 1 })
    const wrapper = mountModal()

    setString(wrapper, 'name', 'bob')
    setNumber(wrapper, 'age', 42)
    setString(wrapper, 'email', 'x@y.z')
    setNumber(wrapper, 'id', 7)
    // role and bio left empty -> raw empty strings
    await flushPromises()

    await saveButton(wrapper)?.trigger('click')
    await flushPromises()

    const modal = wrapper.findComponent(RowModal)
    expect(mockedApi).toHaveBeenCalledWith('POST', rowsPath, {
      name: 'bob',
      age: 42,
      role: '',
      bio: '',
      email: 'x@y.z',
      id: 7,
    })
    expect(modal.emitted('saved')).toHaveLength(1)
    expect(modal.emitted('update:show')).toEqual([[false]])
  })

  it('leaves an untouched numeric insert field as null', async () => {
    mockedApi.mockResolvedValue({ lastInsertId: 1 })
    const wrapper = mountModal()

    setString(wrapper, 'email', 'x@y.z')
    setNumber(wrapper, 'id', 7)
    await flushPromises()

    await saveButton(wrapper)?.trigger('click')
    await flushPromises()

    expect(mockedApi).toHaveBeenCalledWith('POST', rowsPath, {
      name: '',
      age: null,
      role: '',
      bio: '',
      email: 'x@y.z',
      id: 7,
    })
  })

  it('is disabled while a non-nullable field is empty', () => {
    const wrapper = mountModal()

    expect(saveButton(wrapper)?.props('disabled')).toBe(true)
  })

  it('exempts an untouched auto-increment column from the required check and sends null', async () => {
    mockedApi.mockResolvedValue({ lastInsertId: 1 })
    const wrapper = mountModal()

    // Only the genuinely required email is filled; the NOT NULL
    // auto-increment id stays untouched and must not block the save.
    setString(wrapper, 'email', 'x@y.z')
    await flushPromises()

    expect(saveButton(wrapper)?.props('disabled')).toBe(false)
    await saveButton(wrapper)?.trigger('click')
    await flushPromises()

    const modal = wrapper.findComponent(RowModal)
    expect(modal.emitted('saved')).toHaveLength(1)
    expect(mockedApi).toHaveBeenCalledWith('POST', rowsPath, {
      name: '',
      age: null,
      role: '',
      bio: '',
      email: 'x@y.z',
      id: null,
    })
    wrapper.unmount()
  })

  it('patches only the changed fields on edit with where = row pk', async () => {
    mockedApi.mockResolvedValue(undefined)
    const wrapper = mountModal({
      mode: 'edit',
      row: { name: 'old', age: 5, role: 'user', bio: '', email: 'old@x', id: 1 },
      rowPk: { id: 1 },
    })

    setString(wrapper, 'name', 'new')
    await flushPromises()

    expect(saveButton(wrapper)?.props('disabled')).toBe(false)
    await saveButton(wrapper)?.trigger('click')
    await flushPromises()

    const modal = wrapper.findComponent(RowModal)
    expect(mockedApi).toHaveBeenCalledWith('PATCH', rowsPath, {
      where: { id: 1 },
      values: { name: 'new' },
    })
    expect(modal.emitted('saved')).toHaveLength(1)
  })

  it('closes without a request when editing changed nothing', async () => {
    const wrapper = mountModal({
      mode: 'edit',
      row: { name: 'old', age: 5, role: 'user', bio: '', email: 'old@x', id: 1 },
      rowPk: { id: 1 },
    })

    await saveButton(wrapper)?.trigger('click')
    await flushPromises()

    const modal = wrapper.findComponent(RowModal)
    expect(mockedApi).not.toHaveBeenCalled()
    expect(modal.emitted('update:show')).toEqual([[false]])
    expect(modal.emitted('saved')).toBeUndefined()
  })

  it('does not send an untouched decimal that arrived as a string', async () => {
    mockedApi.mockResolvedValue(undefined)
    const cols: ColumnDef[] = [
      { name: 'amount', type: 'decimal', nullable: true },
      { name: 'email', type: 'varchar', nullable: false },
    ]
    const wrapper = mountModal({
      mode: 'edit',
      columns: cols,
      // The backend serializes DECIMAL as a string.
      row: { amount: '123.45', email: 'old@x' },
      rowPk: { id: 1 },
    })

    setString(wrapper, 'email', 'new@x')
    await flushPromises()

    await saveButton(wrapper)?.trigger('click')
    await flushPromises()

    // Only the truly changed field is patched; the coerced decimal is not
    // flagged as changed (and its precision-lost Number is never written).
    expect(mockedApi).toHaveBeenCalledWith('PATCH', rowsPath, {
      where: { id: 1 },
      values: { email: 'new@x' },
    })
  })

  it('sends a decimal the user actually changed, as a number', async () => {
    mockedApi.mockResolvedValue(undefined)
    const cols: ColumnDef[] = [
      { name: 'amount', type: 'decimal', nullable: true },
      { name: 'email', type: 'varchar', nullable: false },
    ]
    const wrapper = mountModal({
      mode: 'edit',
      columns: cols,
      row: { amount: '123.45', email: 'old@x' },
      rowPk: { id: 1 },
    })

    setNumber(wrapper, 'amount', 9.99)
    await flushPromises()

    await saveButton(wrapper)?.trigger('click')
    await flushPromises()

    expect(mockedApi).toHaveBeenCalledWith('PATCH', rowsPath, {
      where: { id: 1 },
      values: { amount: 9.99 },
    })
  })

  it('keeps in-progress input when the columns prop is enriched after the modal opened', async () => {
    const wrapper = mountModal()

    setString(wrapper, 'name', 'typed-before-def-arrived')
    setString(wrapper, 'email', 'x@y.z')
    setNumber(wrapper, 'id', 7)
    await flushPromises()

    // TableDef fetch lands: new columns array (same names, richer metadata).
    wrapper.setProps({ columns: [{ ...columns[0], comment: 'enriched' }, ...columns.slice(1)] })
    await flushPromises()

    await saveButton(wrapper)?.trigger('click')
    await flushPromises()

    expect(mockedApi).toHaveBeenCalledWith('POST', rowsPath, expect.objectContaining({ name: 'typed-before-def-arrived' }))
  })

  it('prefills from a literal column default but not from an SQL expression', () => {
    const cols: ColumnDef[] = [
      {
        name: 'status',
        type: 'varchar',
        nullable: true,
        default: { value: 'active', isSqlExpression: false },
      },
      {
        name: 'created',
        type: 'datetime',
        nullable: true,
        default: { value: 'CURRENT_TIMESTAMP', isSqlExpression: true },
      },
    ]
    const wrapper = mountModal({ columns: cols })

    expect(fieldOf(wrapper, 'status').findComponent(NInput).props('value')).toBe('active')
    expect(fieldOf(wrapper, 'created').findComponent(NInput).props('value')).toBe('')
  })

  it('shows a notification with the ApiError message and keeps the modal open on failure', async () => {
    const { ApiError } = await import('../api/client')
    mockedApi.mockRejectedValue(new ApiError('bad_request', 'cannot insert', 400))
    const wrapper = mountModal()

    setString(wrapper, 'email', 'x@y.z')
    setNumber(wrapper, 'id', 7)
    await flushPromises()

    await saveButton(wrapper)?.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('cannot insert')
    const modal = wrapper.findComponent(RowModal)
    expect(modal.emitted('update:show')).toBeUndefined()
    expect(modal.emitted('saved')).toBeUndefined()
    wrapper.unmount()
  })
})

describe('TableView row modal and stale-response guard', () => {
  const pkUrl = '/api/databases/mydb/tables/users/pk'
  const rowsUrl = '/api/databases/mydb/tables/users/rows'
  const defUrl = '/api/databases/mydb/tables/users'

  const columns: ColumnDef[] = [
    { name: 'id', type: 'int', nullable: false, autoIncrement: true },
    { name: 'name', type: 'varchar', nullable: true },
  ]

  function mockBase(): void {
    mockedApi.mockImplementation(async (method, url) => {
      if (method === 'GET' && url === pkUrl) return { columns: ['id'] }
      if (method === 'GET' && url === defUrl) {
        return { columns, indexes: [], foreignKeys: [], options: {} }
      }
      if (typeof url === 'string' && url.startsWith(rowsUrl)) {
        return {
          columns: [
            { name: 'id', type: 'int', nullable: false },
            { name: 'name', type: 'varchar', nullable: true },
          ],
          rows: [[1, 'alice']],
          totalRows: 1,
        }
      }
      throw new Error(`unexpected api call: ${method} ${url}`)
    })
  }

  const TableViewInProvider = defineComponent({
    setup() {
      return () => h(NNotificationProvider, null, { default: () => h(TableView) })
    },
  })

  async function mountView() {
    const wrapper = mount(TableViewInProvider, { global: { plugins: [createPinia(), i18n] } })
    await flushPromises()
    await flushPromises()
    return wrapper
  }

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('opens the modal in insert mode on add-row with the table definition columns', async () => {
    mockBase()
    const wrapper = await mountView()

    const grid = wrapper.findComponent(DataGrid)
    grid.vm.$emit('add-row')
    await flushPromises()

    const modal = wrapper.findComponent(RowModal)
    expect(modal.exists()).toBe(true)
    expect(modal.props('mode')).toBe('insert')
    expect(modal.props('columns')).toEqual(columns)
    // The def fetch happened for enum values / richer typing.
    expect(mockedApi).toHaveBeenCalledWith('GET', defUrl)
    wrapper.unmount()
  })

  it('opens the modal in edit mode with the row values and pk on edit-row', async () => {
    mockBase()
    const wrapper = await mountView()

    const grid = wrapper.findComponent(DataGrid)
    grid.vm.$emit('edit-row', { id: 1, name: 'alice', __key: '1', __pk: { id: 1 } })
    await flushPromises()

    const modal = wrapper.findComponent(RowModal)
    expect(modal.props('mode')).toBe('edit')
    expect(modal.props('row')).toEqual({ id: 1, name: 'alice' })
    expect(modal.props('rowPk')).toEqual({ id: 1 })
    wrapper.unmount()
  })

  it('refetches rows after the modal reports saved', async () => {
    mockBase()
    const wrapper = await mountView()

    const rowCalls = () =>
      mockedApi.mock.calls.filter(([m, u]) => m === 'GET' && String(u).startsWith(rowsUrl)).length
    const before = rowCalls()

    const modal = wrapper.findComponent(RowModal)
    modal.vm.$emit('saved')
    await flushPromises()
    await flushPromises()

    expect(rowCalls()).toBeGreaterThan(before)
    expect(modal.props('show')).toBe(false)
    wrapper.unmount()
  })

  it('posts row inserts to the rows endpoint, not the table endpoint', async () => {
    // Regression pin: the modal used to receive the bare table path and
    // POST/PATCH against /tables/{t} (chi 405) — the row was never
    // inserted. The modal's path must be the /rows collection.
    mockedApi.mockImplementation(async (method, url) => {
      if (method === 'GET' && url === pkUrl) return { columns: ['id'] }
      if (method === 'GET' && url === defUrl) {
        return { columns, indexes: [], foreignKeys: [], options: {} }
      }
      if (method === 'POST' && url === rowsUrl) return { lastInsertId: 1 }
      if (typeof url === 'string' && url.startsWith(rowsUrl)) {
        return {
          columns: [
            { name: 'id', type: 'int', nullable: false },
            { name: 'name', type: 'varchar', nullable: true },
          ],
          rows: [[1, 'alice']],
          totalRows: 1,
        }
      }
      throw new Error(`unexpected api call: ${method} ${url}`)
    })
    const wrapper = await mountView()

    const grid = wrapper.findComponent(DataGrid)
    grid.vm.$emit('add-row')
    await flushPromises()

    // id is auto-increment (exempt from the required check), name is
    // nullable — Save is enabled with both fields untouched.
    const modal = wrapper.findComponent(RowModal)
    const save = modal.findAllComponents(NButton).find((b) => b.text() === 'Сохранить')
    expect(save?.props('disabled')).toBe(false)
    await save?.trigger('click')
    await flushPromises()

    expect(mockedApi).toHaveBeenCalledWith('POST', rowsUrl, { id: null, name: '' })
    expect(modal.emitted('saved')).toHaveLength(1)
    wrapper.unmount()
  })

  it('falls back to the page ColumnMeta when the table definition fetch fails', async () => {
    mockedApi.mockImplementation(async (method, url) => {
      if (method === 'GET' && url === pkUrl) return { columns: ['id'] }
      if (method === 'GET' && url === defUrl) throw new Error('def failed')
      if (typeof url === 'string' && url.startsWith(rowsUrl)) {
        return {
          columns: [{ name: 'id', type: 'int', nullable: false }],
          rows: [[1]],
          totalRows: 1,
        }
      }
      throw new Error(`unexpected api call: ${method} ${url}`)
    })
    const wrapper = await mountView()

    const grid = wrapper.findComponent(DataGrid)
    grid.vm.$emit('add-row')
    await flushPromises()

    const modal = wrapper.findComponent(RowModal)
    expect(modal.props('columns')).toEqual([{ name: 'id', type: 'int', nullable: false }])
    wrapper.unmount()
  })

  it('ignores a stale rows response that resolves after a newer one', async () => {
    mockBase()
    const wrapper = await mountView()

    // Two controlled promises: the first refetch resolves last.
    let resolveA!: (p: Page) => void
    let resolveB!: (p: Page) => void
    mockedApi.mockImplementation(async (_method, url) => {
      if (url === pkUrl) return { columns: ['id'] }
      if (typeof url === 'string' && url.startsWith(rowsUrl)) {
        if (!resolveA) {
          return new Promise<Page>((res) => {
            resolveA = res
          })
        }
        return new Promise<Page>((res) => {
          resolveB = res
        })
      }
      throw new Error(`unexpected api call: ${_method} ${url}`)
    })

    const grid = wrapper.findComponent(DataGrid)
    grid.vm.$emit('update:page', 2)
    await flushPromises()
    grid.vm.$emit('update:page', 3)
    await flushPromises()

    // The second request (offset 100) resolves first, the stale first
    // request (offset 50) resolves last and must be discarded.
    resolveB({
      columns: [{ name: 'id', type: 'int', nullable: false }],
      rows: [[103]],
      totalRows: 200,
    })
    await flushPromises()
    resolveA({
      columns: [{ name: 'id', type: 'int', nullable: false }],
      rows: [[51]],
      totalRows: 200,
    })
    await flushPromises()

    expect(grid.props('rows')).toEqual([[103]])
    expect(grid.props('loading')).toBe(false)
    wrapper.unmount()
  })
})
