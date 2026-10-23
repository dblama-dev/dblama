import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { SessionInfo } from '../api/types'
import { useSessionStore } from '../stores/session'
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
})
