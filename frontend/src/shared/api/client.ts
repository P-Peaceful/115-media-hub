import { getCsrfToken, setCsrfToken } from '../../features/auth/auth-state'

export class ApiError extends Error {
  constructor(public readonly status: number) {
    super(`HTTP ${status}`)
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
    throw new ApiError(response.status)
  }
  if (response.status === 204 || method === 'HEAD') return undefined
  return response.json() as Promise<T>
}
