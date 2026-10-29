import { getCsrfToken, setCsrfToken } from '../../features/auth/auth-state'

export class ApiError extends Error {
  constructor(public readonly status: number, public readonly message: string, public readonly upstreamCode?: string, public readonly category?: string) {
    super(message)
    this.name = 'ApiError'
  }
}

type RequestOptions = RequestInit & { csrfToken?: string }

// Session and CSRF tokens come from the active AuthProvider, never browser storage.
export async function request<T = unknown>(path: string, options: RequestOptions = {}): Promise<T | undefined> {
  const base = 'http://same-origin.invalid'
  if (!path.startsWith('/') || path.startsWith('//') || path.includes('\\') || new URL(path, base).origin !== base) {
    throw new Error('API requests must use a same-origin absolute path')
  }
  const { csrfToken, ...init } = options
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  const method = (init.method || 'GET').toUpperCase()
  const activeCsrfToken = csrfToken ?? getCsrfToken()
  if (activeCsrfToken && !['GET', 'HEAD', 'OPTIONS'].includes(method)) {
    headers.set('X-CSRF-Token', activeCsrfToken)
  }
  const response = await fetch(path, { ...init, method, headers, credentials: 'same-origin' })
  if (!response.ok) {
    if (response.status === 401) {
      setCsrfToken(null)
      if (typeof window !== 'undefined') window.dispatchEvent(new Event('auth:expired'))
    }
    let message = `HTTP ${response.status}`
    let upstreamCode: string | undefined
    let category: string | undefined
    try {
      const payload: unknown = await response.json()
      if (payload && typeof payload === 'object') {
        const body = payload as { message?: unknown; upstreamCode?: unknown; category?: unknown }
        if (typeof body.message === 'string') message = body.message
        if (typeof body.upstreamCode === 'string') upstreamCode = body.upstreamCode
        if (typeof body.category === 'string') category = body.category
      }
    } catch {
      // Keep the stable HTTP fallback when the server did not return JSON.
    }
    throw new ApiError(response.status, message, upstreamCode, category)
  }
  if (response.status === 204 || method === 'HEAD') return undefined
  return response.json() as Promise<T>
}
