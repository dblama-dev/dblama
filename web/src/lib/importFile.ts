// Multipart upload + NDJSON progress reader for the import endpoint.
// Like downloadExport this bypasses the api() wrapper: the request body
// is multipart and the response is a JSON-lines stream, not one JSON
// document.

import { withSchema } from './schema'
import { ApiError } from '../api/client'

/** Shape of the server error envelope: {"error": {...}}. */
interface ErrorEnvelope {
  error?: {
    code: string
    message: string
    detail?: string
  }
}

/** One record of the import NDJSON stream. */
interface ImportLine {
  done?: number
  total?: number
  ok?: boolean
  error?: string
}

export interface ImportCsvOptions {
  /** Target table; required for csv (validated by the caller). */
  table: string
  /** Field separator, a single character; "" keeps the server default. */
  separator: string
  /** Field-enclosing character (e.g. `"`); "" means none. */
  enclosure: string
  /** Discard the file's first record (column-name header). */
  skipHeader: boolean
}

/**
 * Builds the multipart import form. The option fields are appended
 * BEFORE the file part: the server reads the parts in order and keeps
 * the file part open for the driver, so anything appended after it is
 * ignored. csv-only fields are omitted for sql.
 */
export function buildImportFormData(type: 'sql' | 'csv', file: File, csv: ImportCsvOptions): FormData {
  const form = new FormData()
  form.append('type', type)
  if (type === 'csv') {
    form.append('table', csv.table)
    form.append('separator', csv.separator)
    form.append('enclosure', csv.enclosure)
    form.append('skipHeader', String(csv.skipHeader))
  }
  form.append('file', file)
  return form
}

/**
 * POSTs the import form and consumes the application/x-ndjson response:
 * every {"done":N,"total":M} line calls onProgress, an error line
 * throws (the stream stops there) and the final {"ok":true} resolves.
 * SQL import reports consumed bytes after each executed statement with
 * total=0 until EOF; the caller shows an indeterminate state until then.
 */
export async function runImport(
  db: string,
  form: FormData,
  onProgress: (done: number, total: number) => void,
  schema: string | null = null,
): Promise<void> {
  let res: Response
  try {
    res = await fetch(withSchema(`/api/databases/${encodeURIComponent(db)}/import`, schema, true), {
      method: 'POST',
      headers: { 'X-Requested-With': 'XMLHttpRequest' },
      credentials: 'same-origin',
      body: form,
    })
  } catch {
    throw new ApiError('internal', 'network error', 0)
  }

  if (!res.ok) {
    let payload: ErrorEnvelope
    try {
      payload = (await res.json()) as ErrorEnvelope
    } catch {
      payload = {}
    }
    const err = payload.error
    throw new ApiError(
      err?.code ?? (res.status === 401 ? 'auth_required' : 'internal'),
      err?.message ?? `HTTP ${res.status}`,
      res.status,
      err?.detail,
    )
  }

  if (res.body === null) {
    throw new ApiError('internal', 'empty import response', res.status)
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (value !== undefined) buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    // An unterminated tail chunk is a partial line — keep it buffered
    // unless the stream has ended.
    buffer = done ? '' : (lines.pop() ?? '')
    for (const line of lines) {
      if (line.trim() === '') continue
      const record = JSON.parse(line) as ImportLine
      if (record.error !== undefined && record.error !== '') {
        throw new Error(record.error)
      }
      onProgress(record.done ?? 0, record.total ?? 0)
      if (record.ok === true) return
    }
    if (done) break
  }
}
