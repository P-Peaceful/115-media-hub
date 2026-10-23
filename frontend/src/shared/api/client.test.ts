import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, request } from './client'

afterEach(() => vi.unstubAllGlobals())
describe('request', () => {
  it('reads JSON and sends same-origin session cookies', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response('{"status":"ok"}', { headers: { 'content-type': 'application/json' } }))
    vi.stubGlobal('fetch', fetcher)
    expect(await request('/health/live')).toEqual({ status: 'ok' })
    expect(fetcher.mock.calls[0][1].credentials).toBe('same-origin')
  })
  it('returns no value for an empty response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 204 })))
    expect(await request('/api/v1/example')).toBeUndefined()
  })
  it('reports status without exposing an untrusted server error body', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('private details', { status: 503 })))
    await expect(request('/health/ready')).rejects.toMatchObject({ status: 503, message: 'HTTP 503' })
    await expect(request('/health/ready')).rejects.toBeInstanceOf(ApiError)
  })
  it('adds the explicitly supplied CSRF token to mutation requests', async () => {
    const fetcher = vi.fn().mockResolvedValue(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fetcher)
    await request('/api/v1/example', { method: 'POST', csrfToken: 'test-token' })
    expect(new Headers(fetcher.mock.calls[0][1].headers).get('X-CSRF-Token')).toBe('test-token')
  })
  it('rejects external URLs to keep credentials and CSRF tokens same-origin', async () => {
    await expect(request('https://example.com/api')).rejects.toThrow('same-origin')
    await expect(request('//example.com/api')).rejects.toThrow('same-origin')
  })
})
