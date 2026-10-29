// Blob download helper for the streaming export endpoint. Unlike api()
// (which parses JSON), the export response is a raw dump, so this uses
// fetch directly with the same X-Requested-With header the api client
// sends for the CSRF check.

import { withSchema } from './schema'
import { ApiError } from '../api/client'
import type { ExportRequest } from '../api/types'

/** Shape of the server error envelope: {"error": {...}}. */
interface ErrorEnvelope {
  error?: {
    code: string
    message: string
    detail?: string
  }
}

/**
 * POSTs the export request and hands the streamed dump to the browser
 * as a file download. The filename comes from the Content-Disposition
 * header ("attachment; filename=..."); a missing header falls back to
 * "dump". Throws ApiError for transport failures and for JSON error
 * envelopes (the server validates before any header is written).
 */
export async function downloadExport(db: string, body: ExportRequest, schema: string | null = null): Promise<void> {
  let res: Response
  try {
    res = await fetch(withSchema(`/api/databases/${encodeURIComponent(db)}/export`, schema, true), {
      method: 'POST',
      headers: {
        'X-Requested-With': 'XMLHttpRequest',
        'Content-Type': 'application/json',
      },
      credentials: 'same-origin',
      body: JSON.stringify(body.format === 'sql' ? body : { ...body, csvEncoding: body.csvEncoding ?? 'escaped' }),
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

  const blob = await res.blob()
  const filename = filenameFromDisposition(res.headers.get('Content-Disposition')) ?? 'dump'
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

/** Extracts the filename from a Content-Disposition header, preferring
 * the RFC 5987 filename* form (UTF-8, percent-encoded) over the plain
 * filename form. Returns null when neither is present. */
export function filenameFromDisposition(header: string | null): string | null {
  if (header === null) return null
  const extended = /filename\*=UTF-8''([^;]+)/i.exec(header)
  if (extended !== null) {
    try {
      return decodeURIComponent(extended[1].trim())
    } catch {
      return extended[1].trim()
    }
  }
  const plain = /filename="?([^";]+)"?/i.exec(header)
  return plain !== null ? plain[1].trim() : null
}
