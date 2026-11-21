import type { ColumnMeta, Page, TableDef } from '../api/types'
import { buildRowsQuery } from './tableQuery'
import { withSchema } from './schema'

export interface SearchResult {
  table: string
  column: string
  columns: ColumnMeta[]
  rows: unknown[][]
  totalRows: number
  href: string
}
export interface SearchError { table: string; column?: string; message: string }
export interface SearchProgress { completedTables: number; totalTables: number; searchedColumns: number }
export interface SearchOptions {
  db: string
  schema?: string | null
  tables: string[]
  text: string
  signal?: AbortSignal
  requestTimeoutMs?: number
  onResult: (result: SearchResult) => void
  onError?: (error: SearchError) => void
  onProgress?: (progress: SearchProgress) => void
}

/** The CONTAINS operator binds raw text; the server escapes the LIKE pattern
 * with an explicit ESCAPE '!' independently of engine SQL modes. */
export function searchResultLink(db: string, table: string, column: string, text: string, schema?: string | null): string {
  const query = new URLSearchParams({ view: 'data', filters: JSON.stringify([{ column, op: 'CONTAINS', value: text }]), page: '1', pageSize: '20' })
  if (schema) query.set('schema', schema)
  return `/db/${encodeURIComponent(db)}/table/${encodeURIComponent(table)}?${query}`
}

/** Local cancellable GET, keeping the API client's credentials/CSRF contract.
 * Timeout covers both response headers and body consumption. */
async function searchGet<T>(path: string, signal: AbortSignal | undefined, timeoutMs: number): Promise<T> {
  const controller = new AbortController()
  const abort = () => controller.abort()
  signal?.addEventListener('abort', abort, { once: true })
  if (signal?.aborted) controller.abort()
  let timedOut = false
  const timer = setTimeout(() => { timedOut = true; controller.abort() }, timeoutMs)
  try {
    if (controller.signal.aborted) throw new DOMException('Aborted', 'AbortError')
    const response = await fetch(path, { method: 'GET', headers: { 'X-Requested-With': 'XMLHttpRequest' }, credentials: 'same-origin', signal: controller.signal })
    const payload = await response.json()
    if (!response.ok) throw new Error(payload?.error?.message ?? `HTTP ${response.status}`)
    return payload as T
  } catch (error) {
    if (timedOut && !signal?.aborted) throw new Error('requestTimeout')
    throw error
  } finally {
    clearTimeout(timer)
    signal?.removeEventListener('abort', abort)
  }
}

export async function searchDatabase(options: SearchOptions): Promise<SearchProgress & { cancelled: boolean }> {
  if (!options.text.trim()) throw new Error('emptyQuery')
  if (options.tables.length > 20) throw new Error('tooManyTables')
  if (!options.tables.length) throw new Error('noTables')
  const tables = [...new Set(options.tables)]
  const progress: SearchProgress = { completedTables: 0, totalTables: tables.length, searchedColumns: 0 }
  const timeout = Math.max(1, Math.min(options.requestTimeoutMs ?? 15000, 30000))
  let next = 0
  const report = () => options.onProgress?.({ ...progress })
  const get = <T>(path: string) => searchGet<T>(withSchema(path, options.schema ?? null, !!options.schema), options.signal, timeout)
  report()
  async function worker(): Promise<void> {
    while (!options.signal?.aborted && next < tables.length) {
      const table = tables[next++]!
      const base = `/api/databases/${encodeURIComponent(options.db)}/tables/${encodeURIComponent(table)}`
      try {
        const definition = await get<TableDef>(base)
        for (const column of definition.columns) {
          if (options.signal?.aborted) break
          // Arrays, binary, numeric, JSON and custom types have different
          // operator semantics; restrict the scan to known textual types.
          const type = column.type.trim().toLowerCase()
          if (type.endsWith('[]') || !/^(?:char|character varying|character|varchar|tinytext|text|mediumtext|longtext|bpchar|citext|name)(?:\s*\([^)]*\))?$/.test(type)) continue
          try {
            const query = buildRowsQuery({ limit: 20, offset: 0, sort: null, filters: [{ column: column.name, op: 'CONTAINS', value: options.text }] })
            const page = await get<Page>(`${base}/rows?${query}`)
            if (options.signal?.aborted) break
            progress.searchedColumns++
            if (page.rows.length) options.onResult({ table, column: column.name, columns: page.columns, rows: page.rows.slice(0, 20), totalRows: page.totalRows, href: searchResultLink(options.db, table, column.name, options.text, options.schema) })
            report()
          } catch (error) {
            if (options.signal?.aborted) break
            options.onError?.({ table, column: column.name, message: error instanceof Error ? error.message : String(error) })
          }
        }
      } catch (error) {
        if (!options.signal?.aborted) options.onError?.({ table, message: error instanceof Error ? error.message : String(error) })
      }
      if (!options.signal?.aborted) { progress.completedTables++; report() }
    }
  }
  await Promise.all(Array.from({ length: Math.min(2, tables.length) }, worker))
  return { ...progress, cancelled: !!options.signal?.aborted }
}
