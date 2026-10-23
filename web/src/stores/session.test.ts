import { setActivePinia, createPinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { SessionInfo } from '../api/types'
import { useSessionStore } from './session'

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

const loginForm = { driver: 'mysql', host: 'localhost', port: '3306', user: 'root', password: 'pw' }

// res builds a minimal Response double: the client only reads ok, status
// and json() from it.
function res(status: number, body?: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as Response
}

describe('session store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('fills sessionInfo on successful login', async () => {
    // POST /api/session answers 204 with no body, so the store follows up
    // with GET /api/session to learn about the connection.
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(res(204))
      .mockResolvedValueOnce(res(200, sessionInfo))
    vi.stubGlobal('fetch', fetchMock)

    const store = useSessionStore()
    await store.login(loginForm)

    expect(store.sessionInfo).toEqual(sessionInfo)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('rethrows ApiError and leaves sessionInfo null on failed login', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(res(401, { error: { code: 'auth_failed', message: 'access denied' } })),
    )

    const store = useSessionStore()
    await expect(store.login(loginForm)).rejects.toMatchObject({
      code: 'auth_failed',
      status: 401,
    })
    expect(store.sessionInfo).toBeNull()
  })

  it('treats a 401 from the follow-up GET after login as a failure', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(res(204))
      .mockResolvedValueOnce(res(401, { error: { code: 'auth_required', message: 'authentication required' } }))
    vi.stubGlobal('fetch', fetchMock)

    const store = useSessionStore()
    await expect(store.login(loginForm)).rejects.toMatchObject({ code: 'auth_required' })
    expect(store.sessionInfo).toBeNull()
  })

  it('clears sessionInfo on logout', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(res(200, sessionInfo))
      .mockResolvedValueOnce(res(204))
    vi.stubGlobal('fetch', fetchMock)

    const store = useSessionStore()
    await store.fetchSession()
    expect(store.sessionInfo).not.toBeNull()

    await store.logout()

    expect(store.sessionInfo).toBeNull()
    expect(fetchMock).toHaveBeenLastCalledWith('/api/session', {
      method: 'DELETE',
      headers: { 'X-Requested-With': 'XMLHttpRequest' },
      credentials: 'same-origin',
      body: undefined,
    })
  })

  it('clears sessionInfo locally even when DELETE fails, and rethrows', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(res(200, sessionInfo))
      .mockResolvedValueOnce(res(500, { error: { code: 'internal', message: 'internal error' } }))
    vi.stubGlobal('fetch', fetchMock)

    const store = useSessionStore()
    await store.fetchSession()

    await expect(store.logout()).rejects.toMatchObject({ code: 'internal' })
    expect(store.sessionInfo).toBeNull()
  })
})
