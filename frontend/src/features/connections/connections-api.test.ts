import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../../shared/api/client'
import {
  completeQRSession,
  connectionKeys,
  createQRSession,
  getConnectionSummary,
  getQRSession,
  qrImagePath,
} from './connections-api'

afterEach(() => vi.unstubAllGlobals())

describe('115 connection API boundary', () => {
  it('loads the account summary from the protected same-origin endpoint', async () => {
    const summary = { configured: false }
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify(summary), {
      headers: { 'content-type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetcher)

    await expect(getConnectionSummary()).resolves.toEqual(summary)
    expect(fetcher.mock.calls[0][0]).toBe('/api/admin/connections/115')
    expect(fetcher.mock.calls[0][1].credentials).toBe('same-origin')
    expect(connectionKeys.summary).toEqual(['connections', '115'])
  })

  it('keeps opaque local session IDs encoded and excludes QR credentials from URLs', async () => {
    const sessionId = 'local/session?challenge=must-not-appear'
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ status: 'waiting' }), {
      headers: { 'content-type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetcher)

    expect(qrImagePath(sessionId)).toBe('/api/admin/connections/115/qr-sessions/local%2Fsession%3Fchallenge%3Dmust-not-appear/qr')
    expect(qrImagePath(sessionId)).not.toContain('cookie')
    await getQRSession(sessionId)
    expect(fetcher.mock.calls[0][0]).toBe('/api/admin/connections/115/qr-sessions/local%2Fsession%3Fchallenge%3Dmust-not-appear')
  })

  it('sends only the selected device when creating a QR session', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({ sessionId: 'local-id' }), {
      headers: { 'content-type': 'application/json' },
    }))
    vi.stubGlobal('fetch', fetcher)

    await createQRSession('web')
    const [path, options] = fetcher.mock.calls[0]
    expect(path).toBe('/api/admin/connections/115/qr-sessions')
    expect(JSON.parse(options.body)).toEqual({ device: 'web' })
    expect(options.credentials).toBe('same-origin')
  })

  it.each([401, 403])('preserves safe API status and message for HTTP %s', async (status) => {
    const fetcher = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      category: status === 401 ? 'unauthorized' : 'forbidden',
      message: status === 401 ? 'Session expired' : 'CSRF check failed',
    }), { status, headers: { 'content-type': 'application/json' } }))
    vi.stubGlobal('fetch', fetcher)

    await expect(completeQRSession('local-id')).rejects.toMatchObject({
      name: 'ApiError', status,
      message: status === 401 ? 'Session expired' : 'CSRF check failed',
    } satisfies Partial<ApiError>)
  })
})
