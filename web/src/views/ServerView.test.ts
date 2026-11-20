import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { DatabaseInfo, SessionInfo } from '../api/types'
import i18n from '../i18n'
import { useSessionStore } from '../stores/session'
import ServerView from './ServerView.vue'

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

const databases: DatabaseInfo[] = [
  { name: 'information_schema', sizeBytes: 0, collation: 'utf8mb3_general_ci' },
  { name: 'mydb', sizeBytes: 1572864, collation: 'utf8mb4_0900_ai_ci' },
]

describe('ServerView', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('filters the database catalog without a second request and links to account management', async () => {
    const fetchMock = vi.fn().mockResolvedValue(res(200, { databases }))
    vi.stubGlobal('fetch', fetchMock)
    useSessionStore().sessionInfo = { ...sessionInfo, caps: { ...sessionInfo.caps, users: true } }
    const wrapper = mount(ServerView, { global: { plugins: [i18n] } })
    await flushPromises()
    expect(wrapper.get('a[href="/server/users"]').text()).toContain('Пользователи')
    await wrapper.get('input[type=search]').setValue('MYD')
    expect(wrapper.find('a[href="/db/mydb"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('information_schema')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('lists databases with human-readable sizes and refetches on refresh', async () => {
    const fetchMock = vi.fn().mockResolvedValue(res(200, { databases }))
    vi.stubGlobal('fetch', fetchMock)

    // The router guard only admits /server with a session in the store.
    const store = useSessionStore()
    store.sessionInfo = sessionInfo

    const wrapper = mount(ServerView, { global: { plugins: [i18n] } })
    await flushPromises()

    expect(fetchMock).toHaveBeenCalledWith('/api/databases', expect.objectContaining({ method: 'GET' }))
    const text = wrapper.text()
    expect(text).toContain('mydb')
    expect(text).toContain('1.5 MB')
    expect(text).toContain('utf8mb4_0900_ai_ci')

    const refresh = wrapper.findAll('button').find((button) => button.text() === 'Обновить')
    expect(refresh).toBeDefined()
    fetchMock.mockClear()
    await refresh?.trigger('click')
    await flushPromises()
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })
})
