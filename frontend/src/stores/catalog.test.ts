import { setActivePinia, createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '../api/client'
import type { TableInfo } from '../api/types'
import { useCatalogStore } from './catalog'

vi.mock('../api/client', () => ({
  api: vi.fn(),
}))

const mockedApi = vi.mocked(api)

const tables: TableInfo[] = [
  { name: 'users', rows: 3, sizeBytes: 4096, engine: 'InnoDB', comment: 'people', isView: false },
  { name: 'v_active', rows: 0, sizeBytes: 0, engine: '', comment: '', isView: true },
]

describe('catalog store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mockedApi.mockReset()
  })

  it('fetchTables fills tables and clears error', async () => {
    mockedApi.mockResolvedValueOnce({ tables })

    const store = useCatalogStore()
    await store.fetchTables('appdb')

    expect(mockedApi).toHaveBeenCalledWith('GET', '/api/databases/appdb/tables')
    expect(store.tables).toEqual(tables)
    expect(store.error).toBeNull()
    expect(store.loading).toBe(false)
  })

  it('sets error message and leaves loading false on API failure', async () => {
    mockedApi.mockRejectedValueOnce(new Error('connection refused'))

    const store = useCatalogStore()
    await store.fetchTables('appdb')

    expect(store.tables).toEqual([])
    expect(store.error).toBe('connection refused')
    expect(store.loading).toBe(false)
  })

  it('invalidate refetches the same endpoint', async () => {
    mockedApi
      .mockResolvedValueOnce({ tables: [tables[0]] })
      .mockResolvedValueOnce({ tables })

    const store = useCatalogStore()
    await store.fetchTables('appdb')
    await store.invalidate('appdb')

    expect(mockedApi).toHaveBeenCalledTimes(2)
    expect(mockedApi).toHaveBeenLastCalledWith('GET', '/api/databases/appdb/tables')
    expect(store.tables).toEqual(tables)
  })

  it('clears a previous error on a successful refetch', async () => {
    mockedApi
      .mockRejectedValueOnce(new Error('boom'))
      .mockResolvedValueOnce({ tables })

    const store = useCatalogStore()
    await store.fetchTables('appdb')
    expect(store.error).toBe('boom')

    await store.invalidate('appdb')
    expect(store.error).toBeNull()
    expect(store.tables).toEqual(tables)
  })
  it('discards late table responses after switching schemas', async () => {
    let finish!: (v: unknown) => void
    mockedApi.mockImplementationOnce(() => new Promise(resolve => { finish = resolve }))
      .mockResolvedValueOnce({ tables: [tables[1]] })
    const store = useCatalogStore()
    const old = store.selectSchema('appdb', 'old')
    await store.selectSchema('appdb', 'Odd Schema')
    finish({ tables: [tables[0]] })
    await old
    expect(store.tables).toEqual([tables[1]])
    expect(mockedApi).toHaveBeenLastCalledWith('GET', '/api/databases/appdb/tables?schema=Odd%20Schema')
  })

})
