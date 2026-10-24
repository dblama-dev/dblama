import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '../api/client'
import type { QueryResult } from '../api/types'
import { useConsole } from './useConsole'

vi.mock('../api/client', () => ({
  api: vi.fn(),
}))

const mockedApi = vi.mocked(api)

const DB = 'mydb'
const SERVER_KEY = 'mysql|localhost'

const results: QueryResult[] = [
  { columns: [{ name: '1', type: 'int', nullable: true }], rows: [[1]], seconds: 0.01 },
]

interface Call {
  method: string
  url: string
  body?: unknown
}

const calls: Call[] = []

/** Routes the mocked api by URL and records every call. The explain
 * response and the query response are separate arguments. */
function routeApi(queryResponse: unknown = { results }, explainResponse?: unknown): void {
  mockedApi.mockImplementation(async (method, url, body) => {
    calls.push({ method, url, body })
    if (url.startsWith('/api/databases/') && url.endsWith('/query')) return queryResponse
    if (url === '/api/query') return explainResponse
    if (url.startsWith('/api/query/') && url.endsWith('/cancel')) return undefined
    throw new Error(`unexpected url: ${url}`)
  })
}

function makeConsole() {
  return useConsole({ db: () => DB, serverKey: SERVER_KEY })
}

beforeEach(() => {
  localStorage.clear()
  calls.length = 0
  mockedApi.mockReset()
})

describe('useConsole', () => {
  it('posts the sql with a UUID queryID to the db query endpoint and stores results', async () => {
    routeApi()
    const c = makeConsole()
    c.sql.value = 'SELECT 1'

    const ok = await c.execute()

    expect(ok).toBe(true)
    expect(calls).toHaveLength(1)
    const { method, url, body } = calls[0]
    expect(method).toBe('POST')
    expect(url).toBe('/api/databases/mydb/query')
    const payload = body as { sql: string; queryID: string }
    expect(payload.sql).toBe('SELECT 1')
    // crypto.randomUUID() shape: 8-4-4-4-12 hex digits.
    expect(payload.queryID).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
    )
    // maxRows/timeoutSec are omitted: backend defaults (1000 rows / 60s).
    expect(payload).not.toHaveProperty('maxRows')
    expect(payload).not.toHaveProperty('timeoutSec')
    expect(c.results.value).toEqual(results)
    expect(c.error.value).toBe('')
  })

  it('does not call the api for whitespace-only sql', async () => {
    routeApi()
    const c = makeConsole()
    c.sql.value = '   '

    await c.execute()

    expect(calls).toHaveLength(0)
    expect(c.results.value).toBeNull()
  })

  it('executes the selection when one is given, otherwise the whole text', async () => {
    routeApi()
    const c = makeConsole()
    c.sql.value = 'SELECT 1; SELECT 2'

    await c.execute('SELECT 2')
    expect((calls[0].body as { sql: string }).sql).toBe('SELECT 2')

    await c.execute(undefined)
    expect((calls[1].body as { sql: string }).sql).toBe('SELECT 1; SELECT 2')
  })

  it('stores the error message and renders nothing when the query fails', async () => {
    mockedApi.mockRejectedValue(new Error('You have an error in your SQL syntax'))
    const c = makeConsole()
    c.sql.value = 'SELEC 1'

    const ok = await c.execute()

    expect(ok).toBe(false)
    expect(c.error.value).toBe('You have an error in your SQL syntax')
    expect(c.results.value).toBeNull()
  })

  it('cancels the in-flight query with the same queryID', async () => {
    let resolveQuery: (value: unknown) => void = () => {}
    mockedApi.mockImplementation(async (method, url, body) => {
      calls.push({ method, url, body })
      if (url.startsWith('/api/databases/') && url.endsWith('/query')) {
        await new Promise((resolve) => {
          resolveQuery = resolve
        })
        throw new Error('Query was canceled')
      }
      if (url.startsWith('/api/query/') && url.endsWith('/cancel')) return undefined
      throw new Error(`unexpected url: ${url}`)
    })
    const c = makeConsole()
    c.sql.value = 'SELECT SLEEP(10)'

    const pending = c.execute()
    await vi.waitFor(() => {
      expect(c.executing.value).toBe(true)
      expect(c.queryID.value).not.toBe('')
    })

    await c.cancel()
    const queryID = c.queryID.value
    expect(calls.at(-1)?.url).toBe(`/api/query/${queryID}/cancel`)

    resolveQuery(undefined)
    await pending
    expect(c.error.value).toBe('Query was canceled')
  })

  it('pushes executed sql into the server history and exposes it', async () => {
    routeApi()
    const c = makeConsole()
    c.sql.value = 'SELECT 42'

    await c.execute()

    expect(JSON.parse(localStorage.getItem(`dbm-console-${SERVER_KEY}`) ?? '[]')).toEqual([
      'SELECT 42',
    ])
    expect(c.history.value).toEqual(['SELECT 42'])
  })

  it('cycles history with prev/next, most-recent entry first', async () => {
    routeApi()
    const c = makeConsole()
    c.sql.value = 'SELECT 1'
    await c.execute()
    c.sql.value = 'SELECT 2'
    await c.execute()

    // Nothing was cycled yet: next is a no-op.
    expect(c.historyNext()).toBeNull()
    // Up walks from the most recent entry to older ones.
    expect(c.historyPrev()).toBe('SELECT 2')
    expect(c.historyPrev()).toBe('SELECT 1')
    // Down walks back toward the most recent entry, then restores the
    // pre-cycle draft (the editor text at the first Up: 'SELECT 2').
    expect(c.historyNext()).toBe('SELECT 2')
    expect(c.historyNext()).toBe('SELECT 2')
    // The draft is returned once; afterwards the cycle is over.
    expect(c.historyNext()).toBeNull()
  })

  it('restores the pre-cycle draft when Down walks past the newest entry', async () => {
    routeApi()
    const c = makeConsole()
    c.sql.value = 'SELECT 1'
    await c.execute()
    c.sql.value = 'SELECT 2'
    await c.execute()
    // A typed draft that was never executed.
    c.sql.value = 'SELECT draft'

    expect(c.historyPrev()).toBe('SELECT 2')
    expect(c.historyPrev()).toBe('SELECT 1')
    expect(c.historyNext()).toBe('SELECT 2')
    expect(c.historyNext()).toBe('SELECT draft')
    expect(c.historyNext()).toBeNull()
    // Cycling again snapshots the restored draft anew: one Up to the
    // newest entry, one Down straight back to the draft.
    expect(c.historyPrev()).toBe('SELECT 2')
    expect(c.historyNext()).toBe('SELECT draft')
  })

  it('restarts the cycle when it is reset from outside (e.g. a history pick)', async () => {
    routeApi()
    const c = makeConsole()
    c.sql.value = 'SELECT 1'
    await c.execute()
    c.sql.value = 'SELECT 2'
    await c.execute()

    c.historyPrev()
    c.resetHistoryCycle()

    // The stale position is gone: Down is a no-op again and Up starts
    // from the top of the list.
    expect(c.historyNext()).toBeNull()
    expect(c.historyPrev()).toBe('SELECT 2')
  })

  it('does not restore a stale draft after a failed execute', async () => {
    mockedApi.mockRejectedValue(new Error('syntax error'))
    const c = makeConsole()
    c.sql.value = 'BROKEN'
    await c.execute()

    expect(c.historyPrev()).toBeNull()
  })

  it('returns null from historyPrev when the history is empty', () => {
    const c = makeConsole()

    expect(c.historyPrev()).toBeNull()
  })

  it('sends the explain request to the dedicated /api/query endpoint', async () => {
    routeApi(undefined, { result: results[0] })
    const c = makeConsole()
    c.sql.value = 'SELECT 1'

    const ok = await c.explain()

    expect(ok).toBe(true)
    expect(calls).toHaveLength(1)
    expect(calls[0].url).toBe('/api/query')
    expect(calls[0].body).toEqual({ db: DB, sql: 'SELECT 1' })
    expect(c.results.value).toEqual([results[0]])
  })
})
