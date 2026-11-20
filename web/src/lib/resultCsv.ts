import type { QueryResult } from '../api/types'

function cellText(value: unknown): string {
  if (value === null || value === undefined) return '\\N'
  if (typeof value === 'string') return value.startsWith('\\') ? '\\' + value : value
  if (typeof value === 'boolean') return value ? '1' : '0'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function csvRecord(fields: string[]): string {
  // A blank physical line is skipped by CSV readers. Quote a lone empty
  // field so an empty string in a single-column result survives import.
  if (fields.length === 1 && fields[0] === '') return '""\n'
  return fields.map(value => /[",\r\n]/.test(value) ? '"' + value.replaceAll('"', '""') + '"' : value).join(',') + '\n'
}

export function resultCsv(result: QueryResult): Blob {
  const header = csvRecord((result.columns ?? []).map(column => column.name))
  const records = (result.rows ?? []).map(row => csvRecord(row.map(cellText)))
  return new Blob([header, ...records], { type: 'text/csv;charset=utf-8' })
}
