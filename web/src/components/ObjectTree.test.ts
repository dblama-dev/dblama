import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { NInput, NSelect, NTree } from 'naive-ui'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '../api/client'
import type { TableInfo } from '../api/types'
import i18n from '../i18n'
import { useCatalogStore } from '../stores/catalog'
import { useSessionStore } from '../stores/session'
import ObjectTree from './ObjectTree.vue'

// The tree talks to the api client directly (databases) and through the
// catalog store (tables); both go through the same mocked module.
vi.mock('../api/client', () => ({
  api: vi.fn(),
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushSpy }),
}))

const pushSpy = vi.fn()
const mockedApi = vi.mocked(api)

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

async function mountTree(): ReturnType<typeof mount> {
  const wrapper = mount(ObjectTree, { global: { plugins: [i18n] } })
  await flushPromises()
  return wrapper
}

describe('ObjectTree', () => {
  beforeEach(() => {
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

    const input = wrapper.findComponent(NInput)
    expect(input).toBeTruthy()
    input.vm.$emit('update:value', 'USERS')
    await flushPromises()

    const text = wrapper.text()
    expect(text).toContain('users')
    expect(text).not.toContain('orders')
  })

  it('pushes /db/{db}/table/{name} when a table node is clicked', async () => {
    const wrapper = await mountTree()

    const tree = wrapper.findComponent(NTree)
    expect(tree).toBeTruthy()
    tree.vm.$emit('update:selected-keys', ['table:users'])
    await flushPromises()

    expect(pushSpy).toHaveBeenCalledWith('/db/mydb/table/users')
  })

  it('pushes /db/{db}/console when the console leaf is clicked', async () => {
    const wrapper = await mountTree()

    const tree = wrapper.findComponent(NTree)
    tree.vm.$emit('update:selected-keys', ['console'])
    await flushPromises()

    expect(pushSpy).toHaveBeenCalledWith('/db/mydb/console')
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

    const tree = wrapper.findComponent(NTree)
    tree.vm.$emit('update:selected-keys', ['table:data 100%'])
    await flushPromises()

    expect(pushSpy).toHaveBeenCalledWith('/db/mydb/table/data%20100%25')
  })

  it('reloads tables through the catalog store when the database select changes', async () => {
    const wrapper = await mountTree()
    const catalog = useCatalogStore()

    const select = wrapper.findComponent(NSelect)
    expect(select).toBeTruthy()
    select.vm.$emit('update:value', 'otherdb')
    await flushPromises()

    expect(mockedApi).toHaveBeenCalledWith('GET', '/api/databases/otherdb/tables')
    expect(catalog.tables).toEqual(tables)
  })
})
