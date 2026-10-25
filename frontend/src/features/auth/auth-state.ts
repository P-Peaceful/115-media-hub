let csrfToken: string | null = null

export function setCsrfToken(token: string | null) {
  csrfToken = token
}

export function getCsrfToken() {
  return csrfToken
}

export function safeReturnTo(value: unknown): string {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//') || value.includes('\\')) return '/'
  try {
    const target = new URL(value, 'http://115-media-hub.local')
    if (target.origin !== 'http://115-media-hub.local') return '/'
    return `${target.pathname}${target.search}${target.hash}`
  } catch {
    return '/'
  }
}

export function sessionDelayUntilExpiry(expiresAt: string, now = Date.now()) {
  const expiry = Date.parse(expiresAt)
  if (!Number.isFinite(expiry)) return 0
  return Math.max(0, expiry - now)
}
