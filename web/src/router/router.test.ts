import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { RouterView } from 'vue-router'

import type { SessionInfo } from '../api/types'
import i18n from '../i18n'
import { useSessionStore } from '../stores/session'
import { useCatalogStore } from '../stores/catalog'
import router from './index'

// res builds a minimal Response double: the client only reads ok, status
// and json() from it.
function res(status: number, body?: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as Response
}

const sessionInfo: SessionInfo = {
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
  database: null,
}

describe('router guard', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('redirects /server to /login when there is no session', async () => {
    // The guard probes GET /api/session once; without a cookie it 401s.
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(res(401, { error: { code: 'auth_required', message: 'authentication required' } })),
    )

    await router.push('/server')

    expect(router.currentRoute.value.path).toBe('/login')
  })

  it('probes the session only once across unauthenticated navigations', async () => {
    // After one failed probe the guard must remember the failure (the
    // authChecked flag) and not re-probe on every navigation.
    const fetchMock = vi.fn().mockResolvedValue(
      res(401, { error: { code: 'auth_required', message: 'authentication required' } }),
    )
    vi.stubGlobal('fetch', fetchMock)

    await router.push('/server')
    await router.push('/server')

    expect(router.currentRoute.value.path).toBe('/login')
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('admits /server with a session and redirects /login to /server', async () => {
    const store = useSessionStore()
    store.sessionInfo = sessionInfo

    await router.push('/server')
    expect(router.currentRoute.value.path).toBe('/server')

    await router.push('/login')
    expect(router.currentRoute.value.path).toBe('/server')
  })

  it('resolves /db/:db/table/:t with a session and bounces to /login without one', async () => {
    const store = useSessionStore()
    store.sessionInfo = sessionInfo

    await router.push('/db/mydb/table/users')
    expect(router.currentRoute.value.path).toBe('/db/mydb/table/users')
    expect(router.currentRoute.value.params).toMatchObject({ db: 'mydb', t: 'users' })

    // Encoded identifiers (space, %) decode back to the original name.
    await router.push('/db/mydb/table/data%20100%25')
    expect(router.currentRoute.value.params).toMatchObject({ db: 'mydb', t: 'data 100%' })

    store.sessionInfo = null
    // A push to the very same path would be deduplicated without running
    // the guard, so navigate to another /db path.
    await router.push('/db/otherdb/table/x')
    expect(router.currentRoute.value.path).toBe('/login')
  })

  it('renders the /db/:db placeholder under the same layout', async () => {
    const store = useSessionStore()
    store.sessionInfo = sessionInfo

    await router.push('/db/mydb')
    expect(router.currentRoute.value.path).toBe('/db/mydb')
    expect(router.currentRoute.value.params).toMatchObject({ db: 'mydb' })
  })
})

it('preserves schema until navigation is confirmed and resolves database search', async () => {
  setActivePinia(createPinia())
  useSessionStore().sessionInfo = { ...sessionInfo, caps: { ...sessionInfo.caps, schemas: true } }
  await router.push('/db/app?schema=first')
  const catalog = useCatalogStore()
  expect(catalog.schema).toBe('first')
  const removeGuard = router.beforeResolve(to => to.query.schema === 'blocked' ? false : true)
  try {
    await router.push('/db/app/search?schema=blocked')
    expect(catalog.schema).toBe('first')
    expect(router.currentRoute.value.query.schema).toBe('first')
    await router.push('/db/app/search?schema=second')
    expect(catalog.schema).toBe('second')
    expect(router.currentRoute.value.name).toBe('db-search')
  } finally { removeGuard() }
})

describe('database operations route', () => {
  const operationsPath = '/db/old%20%2F%20%25/operations?schema=Odd%20Schema'
  const fetchMock = vi.fn()

  async function openOperations() {
    const pinia = createPinia()
    setActivePinia(pinia)
    useSessionStore().sessionInfo = {
      ...sessionInfo,
      driver: { ...sessionInfo.driver, name: 'PostgreSQL', flavor: 'postgres', version: '17.0' },
      caps: { ...sessionInfo.caps, schemas: true },
    }
    await router.push(operationsPath)
    expect(router.currentRoute.value.name).toBe('db-operations')
    return mount(RouterView, {
      global: { plugins: [router, pinia, i18n], stubs: { AppSidebar: true, AppTopbar: true } },
    })
  }

  beforeEach(() => {
    i18n.global.locale.value = 'en'
    fetchMock.mockReset().mockImplementation(async (path: string, request: RequestInit) => {
      if (request.method !== 'GET') return res(204)
      return res(200, path === '/api/databases' ? { databases: [] } : { tables: [] })
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    i18n.global.locale.value = 'ru'
  })

  it('renames the current database and returns to the new catalog in the same schema', async () => {
    const wrapper = await openOperations()
    try {
      expect(wrapper.get('h1').text()).toBe('Database actions')
      expect(wrapper.get('h2').text()).toBe('old / %')
      await wrapper.findAll('button').find(button => button.text() === 'Rename database')!.trigger('click')
      await wrapper.get('.database-form input').setValue('new / %')
      await wrapper.get('.database-form').trigger('submit')
      await flushPromises()
      expect(fetchMock).toHaveBeenCalledWith('/api/databases/old%20%2F%20%25', expect.objectContaining({
        method: 'PATCH', body: JSON.stringify({ name: 'new / %' }),
      }))
      expect(router.currentRoute.value.path).toBe('/db/new%20%2F%20%25')
      expect(router.currentRoute.value.query.schema).toBe('Odd Schema')
      expect(useCatalogStore().databaseRevision).toBe(1)
    } finally { wrapper.unmount() }
  })

  it('clears a pending drop form when schema changes, then drops the scoped database', async () => {
    const wrapper = await openOperations()
    try {
      const openDrop = () => wrapper.findAll('button').find(button => button.text() === 'Drop database')!.trigger('click')
      await openDrop()
      await wrapper.get('.database-form input').setValue('old / %')
      await router.push('/db/old%20%2F%20%25/operations?schema=next')
      expect(wrapper.find('.database-form').exists()).toBe(false)
      await openDrop()
      expect(wrapper.get('button[type=submit]').attributes('disabled')).toBeDefined()
      await wrapper.get('.database-form input').setValue('old / %')
      await wrapper.get('.database-form').trigger('submit')
      await flushPromises()
      expect(fetchMock).toHaveBeenCalledWith('/api/databases/old%20%2F%20%25', expect.objectContaining({ method: 'DELETE' }))
      expect(router.currentRoute.value.name).toBe('server')
      expect(useCatalogStore().databaseRevision).toBe(1)
    } finally { wrapper.unmount() }
  })

  it('refreshes the server catalog if a deletion finishes after leaving the operations page', async () => {
    const wrapper = await openOperations()
    const original = { name: 'old / %', sizeBytes: 0, collation: '' }
    let dropped = false
    let finishDrop!: () => void
    fetchMock.mockImplementation(async (path: string, request: RequestInit) => {
      if (request.method === 'DELETE') {
        await new Promise<void>(resolve => { finishDrop = resolve })
        dropped = true
        return res(204)
      }
      return res(200, path === '/api/databases' ? { databases: dropped ? [] : [original] } : { tables: [] })
    })
    try {
      await wrapper.findAll('button').find(button => button.text() === 'Drop database')!.trigger('click')
      await wrapper.get('.database-form input').setValue(original.name)
      await wrapper.get('.database-form').trigger('submit')
      await router.push('/server')
      await flushPromises()
      expect(wrapper.get('.database-table').text()).toContain(original.name)
      finishDrop()
      await flushPromises()
      expect(wrapper.get('.database-table').text()).not.toContain(original.name)
      expect(router.currentRoute.value.name).toBe('server')
    } finally { wrapper.unmount() }
  })
})
