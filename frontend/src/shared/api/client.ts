export class ApiError extends Error {
  constructor(public readonly status: number) {
    super(`HTTP ${status}`)
    this.name = 'ApiError'
  }
}

type RequestOptions = RequestInit & { csrfToken?: string }

// A token must be obtained from the future session implementation, never guessed.
export async function request<T = unknown>(path: string, options: RequestOptions = {}): Promise<T | undefined> {
  const base = 'http://same-origin.invalid'
  if (!path.startsWith('/') || new URL(path, base).origin !== base) {
    throw new Error('API requests must use a same-origin absolute path')
  }
  const { csrfToken, ...init } = options
  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  const method = (init.method || 'GET').toUpperCase()
  if (csrfToken && !['GET', 'HEAD', 'OPTIONS'].includes(method)) {
    headers.set('X-CSRF-Token', csrfToken)
  }
  const response = await fetch(path, { ...init, method, headers, credentials: 'same-origin' })
  if (!response.ok) throw new ApiError(response.status)
  if (response.status === 204 || method === 'HEAD') return undefined
  return response.json() as Promise<T>
}
