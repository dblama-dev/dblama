import { afterEach, describe, expect, it, vi } from 'vitest'

import { ApiError, api } from './client'

// res builds a minimal Response double: the client only reads ok, status
// and json() from it.
function res(status: number, body?: unknown): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as Response
}

describe('api client', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('parses the JSON body on success', async () => {
    const fetchMock = vi.fn().mockResolvedValue(res(200, [{ id: 'mysql', name: 'MySQL', defaultPort: '3306' }]))
    vi.stubGlobal('fetch', fetchMock)

    const result = await api<{ id: string }[]>('GET', '/api/drivers')

    expect(result).toEqual([{ id: 'mysql', name: 'MySQL', defaultPort: '3306' }])
    expect(fetchMock).toHaveBeenCalledWith('/api/drivers', {
      method: 'GET',
      headers: { 'X-Requested-With': 'XMLHttpRequest' },
      credentials: 'same-origin',
      body: undefined,
    })
  })

  it('returns undefined for 204 responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(res(204)))

    await expect(api('POST', '/api/session', { user: 'root' })).resolves.toBeUndefined()
  })

  it('throws ApiError with code auth_required on 401', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        res(401, { error: { code: 'auth_required', message: 'authentication required' } }),
      ),
    )

    const err: ApiError = await api('GET', '/api/session').catch((e: ApiError) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err.code).toBe('auth_required')
    expect(err.message).toBe('authentication required')
    expect(err.status).toBe(401)
  })

  it('maps a bare 401 with a non-JSON body to auth_required', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 401,
        json: () => Promise.reject(new SyntaxError('Unexpected token')),
      } as Response),
    )

    const err: ApiError = await api('GET', '/api/session').catch((e: ApiError) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err.code).toBe('auth_required')
    expect(err.message).toBe('HTTP 401')
    expect(err.status).toBe(401)
  })

  it('carries detail from the error envelope', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        res(500, { error: { code: 'internal', message: 'internal error', detail: 'boom' } }),
      ),
    )

    const err: ApiError = await api('GET', '/api/session').catch((e: ApiError) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err.code).toBe('internal')
    expect(err.detail).toBe('boom')
    expect(err.status).toBe(500)
  })

  it('sends X-Requested-With and Content-Type on mutations', async () => {
    const fetchMock = vi.fn().mockResolvedValue(res(204))
    vi.stubGlobal('fetch', fetchMock)

    const form = { driver: 'mysql', host: 'localhost', port: '3306', user: 'root', password: 'pw' }
    await api('POST', '/api/session', form)

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledWith('/api/session', {
      method: 'POST',
      headers: {
        'X-Requested-With': 'XMLHttpRequest',
        'Content-Type': 'application/json',
      },
      credentials: 'same-origin',
      body: JSON.stringify(form),
    })
  })

  it('throws ApiError with code internal on network failure', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('fetch failed')))

    const err: ApiError = await api('GET', '/api/drivers').catch((e: ApiError) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect(err.code).toBe('internal')
  })
})
