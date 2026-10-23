// Typed fetch wrapper for the /api endpoints. Every request carries the
// X-Requested-With header (required by the server's CSRF check), and every
// error — transport or HTTP — surfaces as an ApiError.

/** Shape of the server error envelope: {"error": {...}}. */
interface ErrorEnvelope {
  error?: {
    code: string
    message: string
    detail?: string
    sqlState?: string
    driverCode?: number
  }
}

export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public status: number,
    public detail?: string,
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export async function api<T>(
  method: 'GET' | 'POST' | 'DELETE' | 'PUT' | 'PATCH',
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = { 'X-Requested-With': 'XMLHttpRequest' }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }

  let res: Response
  try {
    res = await fetch(path, {
      method,
      headers,
      credentials: 'same-origin',
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch {
    // Network-level failure: no HTTP response was received.
    throw new ApiError('internal', 'network error', 0)
  }

  if (res.status === 204) {
    return undefined as T
  }

  let payload: unknown
  try {
    payload = await res.json()
  } catch {
    // Non-JSON body; fall through and build an error from the status.
  }

  if (!res.ok) {
    const envelope = (payload ?? {}) as ErrorEnvelope
    const err = envelope.error
    throw new ApiError(
      err?.code ?? 'internal',
      err?.message ?? `HTTP ${res.status}`,
      res.status,
      err?.detail,
    )
  }

  return payload as T
}
