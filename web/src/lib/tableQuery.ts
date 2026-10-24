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
