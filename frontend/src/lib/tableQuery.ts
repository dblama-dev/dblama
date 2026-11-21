// Query-string assembly for the server-side rows endpoint
// (GET /api/databases/{db}/tables/{t}/rows, Task 8 contract).
//
// URLSearchParams percent-encodes ':' as %3A and spaces as '+', both of
// which the Go side decodes before splitting — so the backend receives the
// raw `col:op:value` strings and splits each on the first two ':' only
// (values may themselves contain ':').

export interface RowsSort {
  column: string
  desc: boolean
}

export interface RowsFilter {
  column: string
  op: string
  value: string
}

export interface RowsQueryState {
  limit: number
  offset: number
  sort: RowsSort | null
  filters: RowsFilter[]
}

// The backend whitelist (sqlgen.AllowedOperators) is case-sensitive; these
// must be sent exactly as written here.
export const ROW_FILTER_OPERATORS: readonly string[] = [
  '=',
  '!=',
  '<',
  '>',
  '<=',
  '>=',
  'LIKE',
  'CONTAINS',
  'NOT LIKE',
  'REGEXP',
  'NOT REGEXP',
  'IN',
  'NOT IN',
  'IS NULL',
  'IS NOT NULL',
]

export function isNullOperator(op: string): boolean {
  return op === 'IS NULL' || op === 'IS NOT NULL'
}

// Selection values are compared by the database. Limit shortcuts to scalar
// types whose wire value round-trips through a bound text argument.
const comparableScalar = /^(?:char|varchar|(?:tiny|medium|long)?text|(?:tiny|small|medium|big)?int|integer|numeric|decimal|float|real|double(?: precision)?|bool(?:ean)?|enum|set|uuid|year)$/i
export function filterForCell(column: string, type: string, value: unknown): RowsFilter | null {
  if (!column || column.includes(':')) return null
  if (value === null) return { column, op: 'IS NULL', value: '' }
  if (!comparableScalar.test(type)) return null
  if (typeof value !== 'string' && typeof value !== 'boolean' && !(typeof value === 'number' && Number.isFinite(value))) return null
  const literal = String(value)
  // Filters are included in both the browser address and the rows request.
  if (literal.length > 2048 || new URLSearchParams({ value: literal }).toString().length > 2048) return null
  return { column, op: '=', value: literal }
}

/** buildRowsQuery serializes the grid state into the query string of the
 * rows endpoint. `filter` is repeatable and emitted in order; IS NULL and
 * IS NOT NULL are sent with a trailing colon and no value (the backend
 * SplitN needs three parts). */
export function buildRowsQuery(state: RowsQueryState): string {
  const params = new URLSearchParams()
  params.set('limit', String(state.limit ?? 50))
  params.set('offset', String(state.offset ?? 0))
  if (state.sort) {
    params.set('order', `${state.sort.column}:${state.sort.desc ? 'desc' : 'asc'}`)
  }
  for (const filter of state.filters ?? []) {
    const value = isNullOperator(filter.op) ? '' : filter.value
    params.append('filter', `${filter.column}:${filter.op}:${value}`)
  }
  return params.toString()
}

export interface TableRouteState { page: number; pageSize: number; filters: RowsFilter[]; sort: RowsSort | null }
export function readTableRouteQuery(query: Record<string, unknown>): TableRouteState {
  const positive = (value: unknown, fallback: number) => typeof value === 'string' && /^\d+$/.test(value) && Number.isSafeInteger(Number(value)) && Number(value) > 0 ? Number(value) : fallback
  let filters: RowsFilter[] = []
  let sort: RowsSort | null = null
  try {
    const parsed: unknown = JSON.parse(String(query.filters ?? '[]'))
    if (Array.isArray(parsed)) filters = parsed.filter((f): f is RowsFilter => !!f && typeof f.column === 'string' && typeof f.value === 'string' && ROW_FILTER_OPERATORS.includes(f.op)).map(f => ({column:f.column,op:f.op,value:isNullOperator(f.op)?'':f.value}))
  } catch { /* Invalid URLs use the default query. */ }
  try {
    const parsed = JSON.parse(String(query.sort ?? 'null'))
    if (parsed && typeof parsed.column === 'string' && typeof parsed.desc === 'boolean') sort = {column:parsed.column,desc:parsed.desc}
  } catch { /* Invalid URLs use the default query. */ }
  return {page:positive(query.page,1),pageSize:Math.min(positive(query.pageSize,50),200),filters,sort}
}
export function writeTableRouteQuery(query: Record<string, unknown>, state: TableRouteState): Record<string, string | null | (string | null)[]> {
  const next = {...query} as Record<string,string | null | (string | null)[]>
  delete next.filters; delete next.sort
  if (state.filters.length) next.filters = JSON.stringify(state.filters)
  if (state.sort) next.sort = JSON.stringify(state.sort)
  next.page = String(state.page); next.pageSize = String(state.pageSize)
  return next
}
export function tableContextLink(db: string, table: string, action: string, schema?: string | null): string {
  const query = new URLSearchParams({table})
  if (schema) query.set('schema',schema)
  return `/db/${encodeURIComponent(db)}/${action}?${query}`
}
