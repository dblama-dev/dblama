import { createPinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, type PropType } from 'vue'
import {
  UiButton,
  UiCheckbox,
  UiInput,
  UiInputNumber,
  UiNotificationProvider,
  UiSelect,
} from '../ui'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { api } from '../api/client'
import type { ColumnDef, Page } from '../api/types'
import i18n from '../i18n'
import RowModal from './RowModal.vue'
import DataGrid from './DataGrid.vue'
import TableView from '../views/TableView.vue'
import { useSessionStore } from '../stores/session'
import type { SessionInfo } from '../api/types'

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
    mode: { type: String as PropType<'insert' | 'edit'>, default: 'insert' },
    columns: { type: Array as PropType<ColumnDef[]>, default: () => columns },
    row: { type: Object, default: null },
    rowPk: { type: Object, default: null },
  },
  setup(p) {
    return () =>
      h(UiNotificationProvider, null, {
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

// Render the row form in place. The built-in Teleport test stub remounts
// its children during updates and discards the active field and its listeners.
const formStubs = {
  teleport: true,
  UiModal: defineComponent({
    props: { show: Boolean },
    setup(props, { slots }) {
      return () => props.show ? h('dialog', { open: true }, [slots.default?.(), slots.footer?.()]) : null
    },
  }),
}
function mountModal(props: Record<string, unknown> = {}, flavor = 'postgres') {
  const pinia = createPinia()
  useSessionStore(pinia).sessionInfo = { driver: { flavor } } as SessionInfo
  return mount(ModalInProvider, {
    props,
    global: { plugins: [pinia, i18n], stubs: formStubs },
  })
}

// Per-field helpers keyed by the column label. UiInputNumber and
// UiSelect embed their own UiInput internally, so fields are addressed through
// their wrapper element, not by a global UiInput index.
function fieldOf(wrapper: ReturnType<typeof mount>, name: string) {
  const field = wrapper
    .findAll('.row-modal__field')
    .find((f) => f.find('.row-modal__label').text() === name)
  expect(field, `field for column ${name}`).toBeDefined()
  return field!
}

function setString(wrapper: ReturnType<typeof mount>, name: string, value: string): void {
  fieldOf(wrapper, name).findComponent(UiInput).vm.$emit('update:value', value)
}

function setNumber(wrapper: ReturnType<typeof mount>, name: string, value: number | null): void {
  fieldOf(wrapper, name).findComponent(UiInputNumber).vm.$emit('update:value', value)
}

function checkNull(wrapper: ReturnType<typeof mount>, name: string): void {
  fieldOf(wrapper, name).findComponent(UiCheckbox).vm.$emit('update:checked', true)
}

function saveButton(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAllComponents(UiButton).find((b) => b.text() === 'Сохранить')
}

describe('RowModal', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('waits for the current binary file and ignores the previous selection', async () => {
    const wrapper = mountModal({ columns: [{ name: 'file', type: 'blob', nullable: true }] })
    const input = wrapper.get('input[type=file]')
    let finishOld!: (data: ArrayBuffer) => void
    const old = { arrayBuffer: () => new Promise<ArrayBuffer>(resolve => { finishOld = resolve }) }
    Object.defineProperty(input.element, 'files', { value: [old], configurable: true })
    await input.trigger('change')
    expect(saveButton(wrapper)?.props('disabled')).toBe(true)
    const current = { arrayBuffer: async () => Uint8Array.of(42).buffer }
    Object.defineProperty(input.element, 'files', { value: [current] })
    await input.trigger('change')
    await flushPromises()
    finishOld(Uint8Array.of(99).buffer)
    await flushPromises()
    await saveButton(wrapper)?.trigger('click')
    expect(mockedApi).toHaveBeenCalledWith('POST', rowsPath, { file: { $binaryHex: '2a' } })
    wrapper.unmount()
  })

  it('keeps the newly opened form intact when an earlier insert finishes', async () => {
    let finish!: () => void
    mockedApi.mockReturnValueOnce(new Promise<void>(resolve => { finish = resolve }))
    const wrapper = mountModal({ columns: [{ name: 'label', type: 'varchar', nullable: false }] })
    setString(wrapper, 'label', 'first')
    await saveButton(wrapper)?.trigger('click')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    setString(wrapper, 'label', 'next')
    finish()
    await flushPromises()
    const modal = wrapper.findComponent(RowModal)
    expect(modal.emitted('update:show')).toBeUndefined()
    expect(fieldOf(wrapper, 'label').findComponent(UiInput).props('value')).toBe('next')
    expect(saveButton(wrapper)?.props('disabled')).toBe(false)
    wrapper.unmount()
  })

  it('renders one typed field per column: number, select, textarea, text', () => {
    const wrapper = mountModal()

    const names = wrapper.findAll('.row-modal__field').map((f) => f.find('.row-modal__label').text())
    expect(names).toEqual(['name', 'age', 'role', 'bio', 'email', 'id'])

    expect(fieldOf(wrapper, 'name').findComponent(UiInput).props('type')).toBe('text')
    expect(fieldOf(wrapper, 'bio').findComponent(UiInput).props('type')).toBe('textarea')

    expect(fieldOf(wrapper, 'age').findComponent(UiInputNumber).exists()).toBe(true)
    expect(fieldOf(wrapper, 'id').findComponent(UiInputNumber).exists()).toBe(true)

    const roleSelect = fieldOf(wrapper, 'role').findComponent(UiSelect)
    expect(roleSelect.exists()).toBe(true)
    expect(roleSelect.props('options')).toEqual([
      { label: 'admin', value: 'admin' },
      { label: 'user', value: 'user' },
    ])
    expect(fieldOf(wrapper, 'role').findComponent(UiInputNumber).exists()).toBe(false)
  })

  it('shows a NULL toggle only for nullable columns', () => {
    const wrapper = mountModal()

    // 4 nullable columns get a toggle; email and id do not.
    expect(wrapper.findAllComponents(UiCheckbox)).toHaveLength(4)
    expect(fieldOf(wrapper, 'email').findComponent(UiCheckbox).exists()).toBe(false)
    expect(fieldOf(wrapper, 'id').findComponent(UiCheckbox).exists()).toBe(false)
    expect(fieldOf(wrapper, 'email').get('.row-modal__required').attributes('title')).toContain('NOT NULL')
  })

  it('disables the input and sends null when the NULL toggle is checked', async () => {
    mockedApi.mockResolvedValue({ lastInsertId: 1 })
    const wrapper = mountModal()

    setString(wrapper, 'name', 'bob')
    await flushPromises()
    checkNull(wrapper, 'name')
    await flushPromises()

    expect(fieldOf(wrapper, 'name').findComponent(UiInput).props('disabled')).toBe(true)

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

  it('allows an explicit empty NOT NULL text value', () => {
    const wrapper = mountModal()

    expect(saveButton(wrapper)?.props('disabled')).toBe(false)
  })

  it('omits an untouched auto-increment column from the insert', async () => {
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

  it('sends a decimal the user actually changed without rounding it', async () => {
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

    setString(wrapper, 'amount', '9.99')
    await flushPromises()

    await saveButton(wrapper)?.trigger('click')
    await flushPromises()

    expect(mockedApi).toHaveBeenCalledWith('PATCH', rowsPath, {
      where: { id: 1 },
      values: { amount: '9.99' },
    })
  })

  it('keeps PostgreSQL interval values as text', async () => {
    mockedApi.mockResolvedValue(undefined)
    const wrapper = mountModal({
      mode: 'edit',
      columns: [{ name: 'duration', type: 'interval', nullable: false }],
      row: { duration: '2 days 03:04:05' },
      rowPk: { id: 1 },
    })
    setString(wrapper, 'duration', '3 days 04:05:06')
    await saveButton(wrapper)?.trigger('click')
    await flushPromises()
    expect(mockedApi).toHaveBeenCalledWith('PATCH', rowsPath, {
      where: { id: 1 },
      values: { duration: '3 days 04:05:06' },
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

    expect(fieldOf(wrapper, 'status').findComponent(UiInput).props('value')).toBe('active')
    expect((fieldOf(wrapper, 'created').find('input').element as HTMLInputElement).value).toBe('')
  })

  it('announces the API error once and keeps the modal open on failure', async () => {
    const { ApiError } = await import('../api/client')
    mockedApi.mockRejectedValue(new ApiError('bad_request', 'cannot insert', 400))
    const wrapper = mountModal()

    setString(wrapper, 'email', 'x@y.z')
    setNumber(wrapper, 'id', 7)
    await flushPromises()

    await saveButton(wrapper)?.trigger('click')
    await flushPromises()

    expect(wrapper.findAll('[role=alert]').map(alert => alert.text())).toEqual(['cannot insert'])
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
      return () => h(UiNotificationProvider, null, { default: () => h(TableView) })
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

    await wrapper.get('[data-testid=table-add-row]').trigger('click')
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

    await wrapper.get('[data-testid=table-add-row]').trigger('click')
    await flushPromises()

    // id is auto-increment (exempt from the required check), name is
    // nullable — Save is enabled with both fields untouched.
    const modal = wrapper.findComponent(RowModal)
    const save = modal.findAllComponents(UiButton).find((b) => b.text() === 'Сохранить')
    expect(save?.props('disabled')).toBe(false)
    await save?.trigger('click')
    await flushPromises()

    expect(mockedApi).toHaveBeenCalledWith('POST', rowsUrl, { name: '' })
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

    await wrapper.get('[data-testid=table-add-row]').trigger('click')
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

it('prefills an insert from copied values while leaving identity empty', async () => {
 const wrapper = mountModal({row:{name:'copied',email:'x@y.z'}})
 await flushPromises()
 expect(fieldOf(wrapper,'name').findComponent(UiInput).props('value')).toBe('copied')
 expect(fieldOf(wrapper,'id').findComponent(UiInputNumber).props('value')).toBe(null)
 wrapper.unmount()
})

it('loads parent key choices using the referenced schema and preserves numeric keys', async () => {
 mockedApi.mockResolvedValue({columns:[{name:'parent_id',type:'int',nullable:false}],rows:[[7],[9]],totalRows:2})
 const pinia=createPinia()
 const wrapper=mount(defineComponent({setup:()=>()=>h(UiNotificationProvider,null,{default:()=>h(RowModal,{show:true,mode:'insert',rowsPath:'/api/databases/mydb/tables/child/rows',columns:[{name:'parent_id',type:'int',nullable:false}],row:null,rowPk:null,foreignKeys:[{name:'fk',columns:['parent_id'],refTable:'parents',refColumns:['parent_id']}]})})}),{global:{plugins:[pinia,i18n],stubs:formStubs}})
 await flushPromises()
 expect(mockedApi).toHaveBeenCalledWith('GET','/api/databases/mydb/tables/parents/rows?limit=200&offset=0')
 const select=wrapper.findComponent(UiSelect)
 expect(select.props('options')).toEqual([{label:'7',value:7},{label:'9',value:9}])
 wrapper.unmount()
})

it('copies exact decimal and binary values without numeric rounding or text reinterpretation', async () => {
 mockedApi.mockResolvedValue({lastInsertId:2})
 const wrapper=mountModal({columns:[{name:'amount',type:'decimal',nullable:false},{name:'data',type:'blob',nullable:false}],row:{amount:'9007199254740993.12',data:'0x00ff'}})
 await flushPromises()
 expect(fieldOf(wrapper,'amount').findComponent(UiInput).props('value')).toBe('9007199254740993.12')
 await saveButton(wrapper)?.trigger('click');await flushPromises()
 expect(mockedApi).toHaveBeenCalledWith('POST',rowsPath,{amount:'9007199254740993.12',data:{$binaryHex:'00ff'}})
 wrapper.unmount()
})

it('allows an existing empty NOT NULL string while PATCHing only changed notes', async () => {
 mockedApi.mockResolvedValue(undefined)
 const wrapper=mountModal({mode:'edit',columns:[{name:'name',type:'varchar',nullable:false},{name:'notes',type:'text',nullable:false}],row:{name:'',notes:''},rowPk:{id:'9007199254740993'}})
 setString(wrapper,'notes','changed')
 await flushPromises()
 expect(saveButton(wrapper)?.props('disabled')).toBe(false)
 await saveButton(wrapper)?.trigger('click'); await flushPromises()
 expect(mockedApi).toHaveBeenCalledWith('PATCH',rowsPath,{where:{id:'9007199254740993'},values:{notes:'changed'}})
 wrapper.unmount()
})

it('omits generated identity and literal or expression defaults on insert', async () => {
 mockedApi.mockResolvedValue({lastInsertId:1})
 const wrapper=mountModal({columns:[{name:'id',type:'bigint',nullable:false,autoIncrement:true},{name:'status',type:'varchar',nullable:false,default:{value:'active'}},{name:'created',type:'timestamp',nullable:false,default:{value:'CURRENT_TIMESTAMP',isSqlExpression:true}},{name:'derived',type:'int',nullable:false,generated:'id + 1'}]})
 await saveButton(wrapper)?.trigger('click');await flushPromises()
 expect(mockedApi).toHaveBeenCalledWith('POST',rowsPath,{})
 wrapper.unmount()
})

it('preserves explicit empty string and NULL instead of server defaults', async () => {
 mockedApi.mockResolvedValue({lastInsertId:1})
 const wrapper=mountModal({columns:[{name:'status',type:'varchar',nullable:false,default:{value:'active'}},{name:'notes',type:'text',nullable:true,default:{value:'note'}}]})
 fieldOf(wrapper,'status').find('.row-modal__mode').findComponent(UiSelect).vm.$emit('update:value','value')
 setString(wrapper,'status','')
 checkNull(wrapper,'notes')
 await flushPromises();await saveButton(wrapper)?.trigger('click');await flushPromises()
 expect(mockedApi).toHaveBeenCalledWith('POST',rowsPath,{status:'',notes:null})
 wrapper.unmount()
})

it('preserves untouched BIGINT, DECIMAL and binary when editing another field',async()=>{
 mockedApi.mockResolvedValue(undefined)
 const wrapper=mountModal({mode:'edit',columns:[{name:'id',type:'bigint',nullable:false},{name:'amount',type:'decimal',nullable:false},{name:'data',type:'bytea',nullable:false},{name:'notes',type:'text',nullable:false}],row:{id:'9007199254740993',amount:'9007199254740993.123',data:'0x00ff',notes:''},rowPk:{id:'9007199254740993'}})
 setString(wrapper,'notes','new');await flushPromises();await saveButton(wrapper)?.trigger('click');await flushPromises()
 expect(mockedApi).toHaveBeenCalledWith('PATCH',rowsPath,{where:{id:'9007199254740993'},values:{notes:'new'}})
 wrapper.unmount()
})

it('shows an actionable JSON validation error without sending invalid input',async()=>{
 const wrapper=mountModal({columns:[{name:'data',type:'json',nullable:false}]})
 setString(wrapper,'data','{bad');await flushPromises();await saveButton(wrapper)?.trigger('click');await flushPromises()
 expect(mockedApi).not.toHaveBeenCalled()
 expect(wrapper.text()).toMatch(/JSON/)
 expect(wrapper.find('.row-modal__error').text()).toMatch(/data/)
 wrapper.unmount()
})

it('searches remote FK keys while keeping the selected exact manual ID available',async()=>{
 mockedApi.mockResolvedValue({columns:[{name:'id',type:'bigint',nullable:false},{name:'name',type:'text',nullable:false}],rows:[['8','Eight']],totalRows:1})
 const wrapper=mount(defineComponent({setup:()=>()=>h(UiNotificationProvider,null,{default:()=>h(RowModal,{show:true,mode:'edit',rowsPath:'/api/databases/mydb/tables/child/rows?schema=public',columns:[{name:'parent_id',type:'bigint',nullable:false}],row:{parent_id:'9007199254740993'},rowPk:{id:1},foreignKeys:[{name:'fk',columns:['parent_id'],refSchema:'other',refTable:'parents',refColumns:['id']}]})})}),{global:{plugins:[createPinia(),i18n],stubs:formStubs}})
 await flushPromises()
 const select=fieldOf(wrapper,'parent_id').findComponent(UiSelect)
 expect(select.props('options')).toContainEqual({label:'9007199254740993',value:'9007199254740993'})
 expect(select.props('options')).toContainEqual({label:'Eight (8)',value:'8'})
 fieldOf(wrapper,'parent_id').find('.row-modal__fk-search-mode').findComponent(UiSelect).vm.$emit('update:value','key')
 select.vm.$emit('search','999')
 await new Promise(resolve=>setTimeout(resolve,350));await flushPromises()
 const searches=mockedApi.mock.calls.filter(([method,url])=>method==='GET'&&new URL(String(url),'http://test').searchParams.has('filter'))
 expect(searches.length).toBeGreaterThan(0)
 expect(new URL(String(searches.at(-1)?.[1]),'http://test').searchParams.get('filter')).toBe('id:=:999')
 wrapper.unmount()
})

it('renders boolean choices with a selected false value and preserves boolean payloads', async()=>{
 mockedApi.mockResolvedValue(undefined)
 const wrapper=mountModal({mode:'edit',columns:[{name:'enabled',type:'boolean',nullable:false}],row:{enabled:false},rowPk:{id:1}})
 const select=fieldOf(wrapper,'enabled').findComponent(UiSelect)
 expect(select.props('value')).toBe('false')
 select.vm.$emit('update:value','true');await flushPromises();await saveButton(wrapper)?.trigger('click');await flushPromises()
 expect(mockedApi).toHaveBeenCalledWith('PATCH',rowsPath,{where:{id:1},values:{enabled:true}})
 wrapper.unmount()
})

it('renders date, time and local timestamp editors without rewriting untouched timestamps',async()=>{
 mockedApi.mockResolvedValue(undefined)
 const wrapper=mountModal({mode:'edit',columns:[{name:'day',type:'date',nullable:false},{name:'clock',type:'time',nullable:false},{name:'created',type:'timestamp',nullable:false}],row:{day:'2026-10-29',clock:'14:15:16',created:'2026-10-29 14:15:16.123456'},rowPk:{id:1}})
 expect(fieldOf(wrapper,'day').find('input').attributes('type')).toBe('date')
 expect(fieldOf(wrapper,'clock').find('input').attributes('type')).toBe('time')
 expect(fieldOf(wrapper,'created').find('input').attributes('type')).toBe('datetime-local')
 await fieldOf(wrapper,'day').find('input').setValue('2026-10-30')
 await saveButton(wrapper)?.trigger('click');await flushPromises()
 expect(mockedApi).toHaveBeenCalledWith('PATCH',rowsPath,{where:{id:1},values:{day:'2026-10-30'}})
 wrapper.unmount()
})

it('builds referenced table navigation with encoded identifiers and PostgreSQL schema',async()=>{
 mockedApi.mockResolvedValue({columns:[],rows:[],totalRows:0})
 const pinia=createPinia()
 const {useCatalogStore}=await import('../stores/catalog')
 useCatalogStore(pinia).schemaAware=true
 const wrapper=mount(defineComponent({setup:()=>()=>h(UiNotificationProvider,null,{default:()=>h(RowModal,{show:true,mode:'insert',rowsPath:'/api/databases/my%20db/tables/child/rows?schema=public',columns:[{name:'parent_id',type:'int',nullable:true}],row:null,rowPk:null,foreignKeys:[{name:'fk',columns:['parent_id'],refSchema:'other schema',refTable:'parent/table',refColumns:['id']}]})})}),{global:{plugins:[pinia,i18n],stubs:formStubs}})
 await flushPromises()
 expect(wrapper.find('.row-modal__related a').attributes('href')).toBe('/db/my%20db/table/parent%2Ftable?schema=other+schema')
 const request=String(mockedApi.mock.calls[0]?.[1])
 expect(new URL(request,'http://test').searchParams.get('schema')).toBe('other schema')
 expect(request).toContain('/my%20db/tables/parent%2Ftable/rows')
 wrapper.unmount()
})

it('uploads exact binary file bytes using binary transport',async()=>{
 mockedApi.mockResolvedValue({lastInsertId:1})
 const wrapper=mountModal({columns:[{name:'data',type:'bytea',nullable:false}]})
 const file=fieldOf(wrapper,'data').find('input[type=file]')
 Object.defineProperty(file.element,'files',{value:[{arrayBuffer:async()=>new Uint8Array([0,255,128]).buffer}]})
 await file.trigger('change');await flushPromises();await saveButton(wrapper)?.trigger('click');await flushPromises()
 expect(mockedApi).toHaveBeenCalledWith('POST',rowsPath,{data:{$binaryHex:'00ff80'}})
 wrapper.unmount()
})

it('does not let an old FK response overwrite a newer search', async () => {
 let oldResponse!: (page: Page) => void
 const parentColumns = [{ name: 'id', type: 'bigint', nullable: false }]
 mockedApi.mockReset()
 mockedApi.mockResolvedValueOnce({ columns: parentColumns, rows: [['initial']], totalRows: 1 })
  .mockImplementationOnce(() => new Promise<Page>(resolve => { oldResponse = resolve }))
  .mockResolvedValue({ columns: parentColumns, rows: [['new']], totalRows: 1 })
 const wrapper = mount(defineComponent({ setup: () => () => h(UiNotificationProvider, null, { default: () => h(RowModal, { show: true, mode: 'insert', rowsPath: '/api/databases/mydb/tables/child/rows', columns: [{ name: 'parent_id', type: 'bigint', nullable: true }], row: null, rowPk: null, foreignKeys: [{ name: 'fk', columns: ['parent_id'], refTable: 'parents', refColumns: ['id'] }] }) }) }), { global: { plugins: [createPinia(), i18n], stubs: formStubs } })
 await flushPromises()
 const select = fieldOf(wrapper, 'parent_id').findComponent(UiSelect)
 select.vm.$emit('search', 'old'); await new Promise(resolve => setTimeout(resolve, 300)); await flushPromises()
 select.vm.$emit('search', 'new'); await new Promise(resolve => setTimeout(resolve, 300)); await flushPromises()
 oldResponse({ columns: parentColumns, rows: [['old']], totalRows: 1 }); await flushPromises()
 expect(select.props('options')).toEqual([{ label: 'new', value: 'new' }])
 wrapper.unmount()
})

it('finds an FK record by name beyond the first 200 rows while preserving the selected exact ID', async () => {
 mockedApi.mockReset()
 const parentColumns = [{ name: 'id', type: 'bigint', nullable: false }, { name: 'name', type: 'varchar', nullable: false }]
 const firstPage = Array.from({ length: 200 }, (_, index) => [String(index + 1), `Person ${index + 1}`])
 const search = 'Wild%_!name'
 mockedApi.mockImplementation(async (method, url) => {
  if (method === 'PATCH') return undefined
  const filter = new URL(String(url), 'http://test').searchParams.get('filter')
  return { columns: parentColumns, rows: filter === `name:CONTAINS:${search}` ? [['9007199254740995', `The ${search} record`]] : filter ? [] : firstPage, totalRows: filter ? 1 : 201 }
 })
 const wrapper = mount(defineComponent({ setup: () => () => h(UiNotificationProvider, null, { default: () => h(RowModal, { show: true, mode: 'edit', rowsPath: '/api/databases/mydb/tables/child/rows', columns: [{ name: 'parent_id', type: 'bigint', nullable: false }], row: { parent_id: '9007199254740993' }, rowPk: { id: 1 }, foreignKeys: [{ name: 'fk', columns: ['parent_id'], refTable: 'parents', refColumns: ['id'] }] }) }) }), { global: { plugins: [createPinia(), i18n], stubs: formStubs } })
 await flushPromises()
 const picker = fieldOf(wrapper, 'parent_id').findComponent(UiSelect)
 expect(picker.props('options')).not.toContainEqual(expect.objectContaining({ value: '9007199254740995' }))
 picker.vm.$emit('search', search)
 await new Promise(resolve => setTimeout(resolve, 300)); await flushPromises()
 const requests = mockedApi.mock.calls.filter(([method, url]) => method === 'GET' && new URL(String(url), 'http://test').searchParams.has('filter'))
 expect(new URL(String(requests.at(-1)?.[1]), 'http://test').searchParams.get('filter')).toBe(`name:CONTAINS:${search}`)
 expect(picker.props('options')).toEqual([{ label: '9007199254740993', value: '9007199254740993' }, { label: `The ${search} record (9007199254740995)`, value: '9007199254740995' }])
 expect(picker.props('value')).toBe('9007199254740993')
 picker.vm.$emit('update:value', '9007199254740995')
 await flushPromises(); await saveButton(wrapper)?.trigger('click'); await flushPromises()
 expect(mockedApi).toHaveBeenCalledWith('PATCH', '/api/databases/mydb/tables/child/rows', { where: { id: 1 }, values: { parent_id: '9007199254740995' } })
 wrapper.unmount()
})

it('displays PostgreSQL RFC3339 DATE values without changing the original payload snapshot', async () => {
 mockedApi.mockReset(); mockedApi.mockResolvedValue(undefined)
 const wrapper = mountModal({ mode: 'edit', columns: [{ name: 'day', type: 'date', nullable: false }, { name: 'notes', type: 'text', nullable: false }], row: { day: '2026-10-29T00:00:00Z', notes: '' }, rowPk: { id: 1 } })
 expect((fieldOf(wrapper, 'day').find('input[type=date]').element as HTMLInputElement).value).toBe('2026-10-29')
 setString(wrapper, 'notes', 'changed')
 await flushPromises(); await saveButton(wrapper)?.trigger('click'); await flushPromises()
 expect(mockedApi).toHaveBeenCalledWith('PATCH', rowsPath, { where: { id: 1 }, values: { notes: 'changed' } })
 wrapper.unmount()
})

it.each(['mysql', 'mariadb'])('keeps %s TIME durations visible, editable and insertable without submitting untouched values', async flavor => {
 const durationColumns = [{ name: 'elapsed', type: 'time(6)', nullable: false }, { name: 'notes', type: 'text', nullable: false }]
 for (const elapsed of ['36:00:00.123456', '-01:30:00.000001']) {
  mockedApi.mockReset(); mockedApi.mockResolvedValue(undefined)
  const wrapper = mountModal({ mode: 'edit', columns: durationColumns, row: { elapsed, notes: '' }, rowPk: { id: 1 } }, flavor)
  const duration = fieldOf(wrapper, 'elapsed').get('input')
  expect(duration.attributes('type')).toBe('text')
  expect((duration.element as HTMLInputElement).value).toBe(elapsed)
  setString(wrapper, 'notes', 'changed'); await flushPromises()
  await saveButton(wrapper)!.trigger('click'); await flushPromises()
  expect(mockedApi).toHaveBeenCalledWith('PATCH', rowsPath, { where: { id: 1 }, values: { notes: 'changed' } })
  mockedApi.mockClear()
  await duration.setValue('-36:10:11.123456')
  await saveButton(wrapper)!.trigger('click'); await flushPromises()
  expect(mockedApi).toHaveBeenCalledWith('PATCH', rowsPath, { where: { id: 1 }, values: { elapsed: '-36:10:11.123456', notes: 'changed' } })
  wrapper.unmount()
 }
 mockedApi.mockClear()
 const insert = mountModal({ columns: durationColumns }, flavor)
 await fieldOf(insert, 'elapsed').get('input').setValue('48:01:02.123456')
 await saveButton(insert)!.trigger('click'); await flushPromises()
 expect(mockedApi).toHaveBeenCalledWith('POST', rowsPath, { elapsed: '48:01:02.123456', notes: '' })
 insert.unmount()
})
