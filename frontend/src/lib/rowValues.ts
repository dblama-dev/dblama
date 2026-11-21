import type { ColumnDef } from '../api/types'

export type RowValue = string | number | boolean | null
export type RowMode = 'value' | 'null' | 'default'
export interface RowField { value: RowValue; mode: RowMode }
export function isRowNumeric(type: string): boolean {
  const t = type.toLowerCase()
  return !t.endsWith('[]') && !/^(bigint|int8|decimal|numeric|interval)/.test(t) && /^(int|smallint|tinyint|mediumint|float|double|real)/.test(t)
}
export function isRowBinary(type: string): boolean { return /binary|blob|^bytea$/i.test(type) }
export function isRowJSON(type: string): boolean { return /^jsonb?$/i.test(type) }
export function isRowBoolean(type: string): boolean { return /^(boolean|bool)$/i.test(type) }
export function rowField(col: ColumnDef, row: Record<string, unknown> | null, insert: boolean): RowField {
  const original = row?.[col.name]
  if (original !== undefined && !(insert && col.autoIncrement)) {
    if (original === null) return { value: '', mode: col.nullable ? 'null' : 'value' }
    const value = typeof original === 'boolean' || typeof original === 'number' || typeof original === 'string' ? original : JSON.stringify(original)
    return { value: isRowNumeric(col.type) ? Number(value) : value, mode: 'value' }
  }
  const def = col.default
  return { value: def && !def.isSqlExpression ? def.value : isRowNumeric(col.type) ? null : '', mode: insert && (col.autoIncrement || col.generated || def != null) ? 'default' : 'value' }
}
export function rowTransport(col: ColumnDef, field: RowField): unknown {
  if (field.mode === 'null') return null
  if (isRowBinary(col.type) && typeof field.value === 'string' && /^0x(?:[0-9a-f]{2})*$/i.test(field.value)) return { $binaryHex: field.value.slice(2) }
  if (isRowNumeric(col.type) && field.value === '') return null
  return field.value
}
export function rowPayload(columns: ColumnDef[], fields: Record<string, RowField>, original?: Record<string, unknown>): Record<string, unknown> {
  const values: Record<string, unknown> = {}
  for (const col of columns) {
    const field = fields[col.name]
    if (!field || col.generated || field.mode === 'default') continue
    const value = rowTransport(col, field)
    if (!original || JSON.stringify(value) !== JSON.stringify(original[col.name])) values[col.name] = value
  }
  return values
}
export function rowJSONError(columns: ColumnDef[], fields: Record<string, RowField>, original?: Record<string, unknown>): string | null {
  const changed = rowPayload(columns, fields, original)
  for (const col of columns) {
    if (!isRowJSON(col.type) || !(col.name in changed) || changed[col.name] === null) continue
    try { JSON.parse(String(changed[col.name])) } catch { return col.name }
  }
  return null
}
/** Return the column whose form state cannot represent a valid explicit value. */
export function rowValueError(columns: ColumnDef[], fields: Record<string, RowField>): string | null {
  for (const col of columns) {
    if (col.generated) continue
    const field = fields[col.name]
    if (!field) return col.name
    if (field.mode === 'default') continue
    if (field.mode === 'null') { if (!col.nullable) return col.name; continue }
    if (!col.nullable && field.value === null) return col.name
    if (!col.nullable && /^(bigint|int8|decimal|numeric)/i.test(col.type) && field.value === '') return col.name
    if (isRowNumeric(col.type) && !col.nullable && (field.value === '' || !Number.isFinite(Number(field.value)))) return col.name
    if (isRowBoolean(col.type) && typeof field.value !== 'boolean') return col.name
  }
  return null
}
