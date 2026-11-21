import { enableAutoUnmount, flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { reactive } from 'vue'
import { UiInput, UiSelect, UiTree } from '../ui'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '../api/client'
import type { TableInfo } from '../api/types'
import i18n from '../i18n'
import { useCatalogStore } from '../stores/catalog'
import { useSessionStore } from '../stores/session'
import ObjectTree from './ObjectTree.vue'
import AppSidebar from './AppSidebar.vue'

// The tree talks to the api client directly (databases) and through the
// catalog store (tables); both go through the same mocked module.
vi.mock('../api/client', () => ({
  api: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushSpy }),
  useRoute: () => route,
}))

const pushSpy = vi.fn()
const route = reactive({ params: {} as { db?: string;t?:string },path:'' })
const mockedApi = vi.mocked(api)
enableAutoUnmount(afterEach)

const databasesPayload = {
  databases: [
    { name: 'mydb', sizeBytes: 0, collation: 'utf8mb4_general_ci' },
    { name: 'otherdb', sizeBytes: 0, collation: 'utf8mb4_general_ci' },
  ],
}

const tables: TableInfo[] = [
  { name: 'users', rows: 3, sizeBytes: 4096, engine: 'InnoDB', comment: '', isView: false },
  { name: 'orders', rows: 5, sizeBytes: 8192, engine: 'InnoDB', comment: '', isView: false },
  { name: 'v_active', rows: 0, sizeBytes: 0, engine: '', comment: '', isView: true },
  { name: 'data 100%', rows: 1, sizeBytes: 1024, engine: 'InnoDB', comment: '', isView: false },
]

/** Routes the mocked api by URL: databases list vs tables of a db. */
function mockApi(): void {
  mockedApi.mockImplementation(async (_method, url) => {
    if (url === '/api/databases') return databasesPayload
    if (url.startsWith('/api/databases/') && url.endsWith('/tables')) return { tables }
    throw new Error(`unexpected url: ${url}`)
  })
}

async function mountTree(): Promise<ReturnType<typeof mount>> {
  const wrapper = mount(ObjectTree, { global: { plugins: [i18n], stubs: { RouterLink: RouterLinkStub } } })
  await flushPromises()
  return wrapper
}

describe('ObjectTree', () => {
  beforeEach(() => {
    route.params = {}
    setActivePinia(createPinia())
    pushSpy.mockReset()
    mockApi()
    const session = useSessionStore()
    session.sessionInfo = {
      driver: { name: 'MySQL', flavor: 'mysql', version: '8.0.36', user: 'root', host: 'localhost' },
      caps: {
        views: true,
        triggers: true,
        routines: true,
        procedures: true,
        events: false,
        partitions: false,
        sequences: false,
        schemas: false,
        users: false,
        monitor: false,
        moveCopy: false,
        comments: true,
        upsert: false,
        descendingIndexes: false,
      },
      database: 'mydb',
    }
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('preselects the session database and renders tables grouped into tables and views', async () => {
    const wrapper = await mountTree()

    // Session database mydb is preselected and its tables are loaded.
    expect(mockedApi).toHaveBeenCalledWith('GET', '/api/databases/mydb/tables')
    const text = wrapper.text()
    expect(text).toContain('users')
    expect(text).toContain('orders')
    // Views go to their own group; the group exists because views exist.
    expect(text).toContain('Таблицы')
    expect(text).toContain('Представления')
    expect(text).toContain('v_active')
  })

  it('filters tables by a case-insensitive substring', async () => {
    const wrapper = await mountTree()

    const input = wrapper.findComponent(UiInput)
    expect(input).toBeTruthy()
    input.vm.$emit('update:value', 'USERS')
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('users')
    expect(text).not.toContain('orders')
  })

  it('shows the route database tables instead of replacing them with the login database', async () => {
    route.params.db = 'otherdb'
    mockedApi.mockImplementation(async (_method, url) => {
      if (url === '/api/databases') return databasesPayload
      if (url === '/api/databases/mydb/tables') return { tables }
      if (url === '/api/databases/otherdb/tables') return { tables: [{ ...tables[0], name: 'other_records' }] }
      throw new Error(`unexpected url: ${url}`)
    })
    const wrapper = await mountTree()
    expect(wrapper.findComponent(UiSelect).props('value')).toBe('otherdb')
    expect(wrapper.text()).toContain('other_records')
    expect(wrapper.text()).not.toContain('users')
  })

  it('keeps a newly renamed route database when an older database-list refresh finishes', async () => {
    const wrapper = await mountTree()
    let finishList!: (value: typeof databasesPayload) => void
    mockedApi.mockImplementation(async (_method, url) => {
      if (url === '/api/databases') return new Promise(resolve => { finishList = resolve })
      if (url === '/api/databases/mydb/tables') return { tables }
      if (url === '/api/databases/renamed/tables') return { tables: [{ ...tables[0], name: 'renamed_records' }] }
      throw new Error(`unexpected url: ${url}`)
    })
    useCatalogStore().databaseRevision++
    await flushPromises()
    route.params.db = 'renamed'
    await flushPromises()
    expect(wrapper.text()).toContain('renamed_records')
    finishList(databasesPayload)
    await flushPromises()
    expect(wrapper.findComponent(UiSelect).props('value')).toBe('renamed')
    expect(wrapper.text()).toContain('renamed_records')
    expect(wrapper.text()).not.toContain('users')
  })

  it('pushes /db/{db}/table/{name} when a table node is clicked', async () => {
    const wrapper = await mountTree()

    const tree = wrapper.findComponent(UiTree)
    expect(tree).toBeTruthy()
    tree.vm.$emit('update:selected-keys', ['table:users'])
    await flushPromises()

    expect(pushSpy).toHaveBeenCalledWith('/db/mydb/table/users')
  })

  it('provides searchable database selection and one set of scoped tools above the object tree', async () => {
    const session = useSessionStore()
    session.sessionInfo!.caps.schemas = true
    session.sessionInfo!.caps.users = true
    session.sessionInfo!.caps.monitor = true
    route.params.db = 'otherdb'
    useCatalogStore().schema = 'Odd Schema'
    mockedApi.mockImplementation(async (_method, path) => path === '/api/schemas' ? { schemas: ['Odd Schema'] } : path === '/api/databases' ? databasesPayload : { tables })
    const wrapper = mount(AppSidebar, { global: { plugins: [i18n], stubs: { RouterLink: RouterLinkStub } } })
    await flushPromises()
    expect(wrapper.findComponent(UiSelect).props('filterable')).toBe(true)
    const destinations = wrapper.findAllComponents(RouterLinkStub).map(link => link.props('to'))
    const groups = wrapper.findComponent(UiTree).props('data')
    expect(groups?.map(group => group.key)).toEqual(['group:tables', 'group:views'])
    for (const action of ['console', 'search', 'import', 'export']) {
      expect(destinations.filter(to => to === `/db/otherdb/${action}?schema=Odd%20Schema`)).toHaveLength(1)
    }
    const more = wrapper.get('details.advanced-navigation')
    expect(more.attributes('open')).toBeUndefined()
    const secondary = wrapper.findAllComponents(RouterLinkStub).filter(link => more.element.contains(link.element)).map(link => link.props('to'))
    expect(secondary).toContain('/server/users')
    expect(secondary).toContain('/server/processes')
    expect(secondary).toContain('/db/otherdb/objects/views?schema=Odd%20Schema')
    expect(secondary).not.toContain('/db/otherdb/objects/events?schema=Odd%20Schema')
  })

  it('pushes the _new structure route when the tables group "+" button is clicked', async () => {
    const wrapper = await mountTree()

    const add = wrapper.find('[data-testid="tree-add-table"]')
    expect(add.exists()).toBe(true)
    await add.trigger('click')
    await flushPromises()

    expect(pushSpy).toHaveBeenCalledWith('/db/mydb/table/_new?view=structure')
  })

  it('URL-encodes identifiers with special characters in navigation', async () => {
    // Names like "data 100%" are legal MySQL identifiers; %, ? and # would
    // break or truncate the path if pushed raw.
    const wrapper = await mountTree()

    const tree = wrapper.findComponent(UiTree)
    tree.vm.$emit('update:selected-keys', ['table:data 100%'])
    await flushPromises()

    expect(pushSpy).toHaveBeenCalledWith('/db/mydb/table/data%20100%25')
  })

  it('reloads tables through the catalog store when the database select changes', async () => {
    const wrapper = await mountTree()
    const catalog = useCatalogStore()

    const select = wrapper.findComponent(UiSelect)
    expect(select).toBeTruthy()
    select.vm.$emit('update:value', 'otherdb')
    await flushPromises()

    expect(mockedApi).toHaveBeenCalledWith('GET', '/api/databases/otherdb/tables')
    expect(catalog.tables).toEqual(tables)
    expect(pushSpy).toHaveBeenCalledWith('/db/otherdb')
  })
  it('selects PostgreSQL schemas and exposes sequences, preserving schema in navigation', async () => {
    const session = useSessionStore()
    session.sessionInfo!.caps.schemas = true
    session.sessionInfo!.caps.sequences = true
    session.sessionInfo!.driver.flavor = 'postgres'
    mockedApi.mockImplementation(async (_method, url) => {
      if (url === '/api/databases') return databasesPayload
      if (url === '/api/schemas') return { schemas: ['public', 'Odd Schema'] }
      if (url.startsWith('/api/sequences')) return { sequences: [{ name: 'order_seq', lastValue: 0, increment: 1 }] }
      if (url.includes('/tables')) return { tables }
      return undefined
    })
    const wrapper = await mountTree()
    expect(mockedApi).toHaveBeenCalledWith('GET', '/api/databases/mydb/tables?schema=public')
    expect(wrapper.find('[data-testid="schema-select"]').exists()).toBe(true)
    wrapper.findAllComponents(UiSelect).find(select => select.attributes('data-testid') === 'schema-select')!.vm.$emit('update:value', 'Odd Schema')
    await flushPromises()
    expect(mockedApi).toHaveBeenCalledWith('GET', '/api/databases/mydb/tables?schema=Odd%20Schema')
    expect(mockedApi).toHaveBeenCalledWith('GET', '/api/sequences?schema=Odd%20Schema')
    wrapper.findComponent(UiTree).vm.$emit('update:selected-keys', ['table:users'])
    await flushPromises()
    expect(pushSpy).toHaveBeenCalledWith('/db/mydb/table/users?schema=Odd%20Schema')
    expect(wrapper.text()).toContain('order_seq')
    // Browser history changes catalog scope through the route guard.
    useCatalogStore().schema = 'public'
    await flushPromises()
    expect(mockedApi).toHaveBeenLastCalledWith('GET', '/api/sequences?schema=public')
    expect(wrapper.find('[data-testid="tree-add-sequence"]').exists()).toBe(true)
    wrapper.unmount()
  })


it('derives the selected table from navigation and browser history',async()=>{
 route.params={db:'mydb',t:'orders'}
 const wrapper=await mountTree()
 expect(wrapper.findComponent(UiTree).props('selectedKeys')).toEqual(['table:orders'])
 route.params.t='users';await flushPromises()
 expect(wrapper.findComponent(UiTree).props('selectedKeys')).toEqual(['table:users'])
 wrapper.unmount()
})
it('keeps database selection and catalog unchanged when navigation is cancelled',async()=>{
 const wrapper=await mountTree();mockedApi.mockClear()
 pushSpy.mockResolvedValueOnce({type:4})
 wrapper.findComponent(UiSelect).vm.$emit('update:value','otherdb');await flushPromises()
 expect(wrapper.findComponent(UiSelect).props('value')).toBe('mydb')
 expect(mockedApi).not.toHaveBeenCalledWith('GET','/api/databases/otherdb/tables')
 wrapper.unmount()
})
it('keeps schema unchanged when navigation is cancelled',async()=>{
 useSessionStore().sessionInfo!.caps.schemas=true
 mockedApi.mockImplementation(async (_method,url)=>url==='/api/databases'?databasesPayload:url==='/api/schemas'?{schemas:['public','other']}:{tables})
 const wrapper=await mountTree();mockedApi.mockClear()
 pushSpy.mockResolvedValueOnce({type:4})
 wrapper.findAllComponents(UiSelect).find(select=>select.attributes('data-testid')==='schema-select')!.vm.$emit('update:value','other');await flushPromises()
 expect(useCatalogStore().schema).toBe('public')
 expect(mockedApi).not.toHaveBeenCalledWith('GET','/api/databases/mydb/tables?schema=other')
 wrapper.unmount()
})

it('separates favorite tables for different connection ports',async()=>{
 localStorage.clear()
 const session=useSessionStore()
 session.sessionInfo!.driver.port='3306'
 route.params={db:'mydb',t:'users'}
 const wrapper=await mountTree()
 await wrapper.get('button[aria-label$=": users"]').trigger('click');await flushPromises()
 expect(wrapper.find('[data-favorite-tables]').exists()).toBe(true)
 session.sessionInfo!.driver.port='3307';await flushPromises()
 expect(wrapper.find('[data-favorite-tables]').exists()).toBe(false)
 session.sessionInfo!.driver.port='3306';await flushPromises()
 expect(wrapper.find('[data-favorite-tables]').exists()).toBe(true)
 wrapper.unmount();localStorage.clear()
})

})
