import { afterEach, describe, expect, it, vi } from 'vitest'
import { createDatabase, renameDatabase, dropDatabase, transferTables, validDatabaseName } from './databaseOps'

afterEach(() => vi.unstubAllGlobals())
describe('database mutations', () => {
  it('encodes source identifiers without changing the requested database name', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetcher)
    await createDatabase('new db', 'utf8mb4_unicode_ci')
    await renameDatabase('odd/db%', 'target')
    await dropDatabase('odd/db%')
    await transferTables('odd/db%', ['one'], 'target', 'copy')
    expect(fetcher.mock.calls.map(([path, init]) => [path, init.method, init.body])).toEqual([
      ['/api/databases', 'POST', '{"name":"new db","collation":"utf8mb4_unicode_ci"}'],
      ['/api/databases/odd%2Fdb%25', 'PATCH', '{"name":"target"}'],
      ['/api/databases/odd%2Fdb%25', 'DELETE', undefined],
      ['/api/databases/odd%2Fdb%25/table-transfer', 'POST', '{"action":"copy","tables":["one"],"targetDB":"target"}'],
    ])
  })
  it('does not issue a mutation for empty names or invalid transfer selections', async () => {
    const fetcher = vi.fn()
    vi.stubGlobal('fetch', fetcher)
    expect(validDatabaseName(' \t')).toBe(false)
    expect(validDatabaseName('a\0b')).toBe(false)
    expect(validDatabaseName('Mixed DB')).toBe(true)
    await expect(createDatabase(' ')).rejects.toThrow()
    await expect(renameDatabase('a', 'a')).rejects.toThrow()
    await expect(transferTables('a', [], 'b', 'move')).rejects.toThrow()
    await expect(transferTables('a', ['t', 't'], 'b', 'copy')).rejects.toThrow()
    await expect(transferTables('a', ['t'], 'a', 'copy')).rejects.toThrow()
    expect(fetcher).not.toHaveBeenCalled()
  })
})
