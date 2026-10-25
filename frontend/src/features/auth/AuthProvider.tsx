import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { ApiError, request } from '../../shared/api/client'
import { sessionDelayUntilExpiry, setCsrfToken } from './auth-state'
import { AuthContext, type AuthStatus } from './auth-context'

type SessionResponse = { authenticated: boolean; csrfToken?: string; expiresAt?: string }
type LoginResponse = { authenticated: boolean; csrfToken: string; expiresAt: string }

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>('loading')
  const expiryTimer = useRef<number | null>(null)

  const clearExpiry = useCallback(() => {
    if (expiryTimer.current !== null) window.clearTimeout(expiryTimer.current)
    expiryTimer.current = null
  }, [])

  const expire = useCallback(() => {
    clearExpiry()
    setCsrfToken(null)
    setStatus('unauthenticated')
  }, [clearExpiry])

  const acceptSession = useCallback((csrfToken: string, expiresAt: string) => {
    clearExpiry()
    const delay = sessionDelayUntilExpiry(expiresAt)
    if (delay === 0) {
      expire()
      return false
    }
    setCsrfToken(csrfToken)
    setStatus('authenticated')
    expiryTimer.current = window.setTimeout(expire, delay)
    return true
  }, [clearExpiry, expire])

  const reload = useCallback(async () => {
    setStatus('loading')
    try {
      const result = await request<SessionResponse>('/api/auth/session')
      if (!result?.authenticated || !result.csrfToken || !result.expiresAt || !acceptSession(result.csrfToken, result.expiresAt)) {
        expire()
      }
    } catch {
      setStatus('error')
    }
  }, [acceptSession, expire])

  useEffect(() => {
    const timer = window.setTimeout(() => { void reload() }, 0)
    const expired = () => expire()
    window.addEventListener('auth:expired', expired)
    return () => {
      window.clearTimeout(timer)
      clearExpiry()
      window.removeEventListener('auth:expired', expired)
    }
  }, [clearExpiry, expire, reload])

  const signIn = useCallback(async (username: string, password: string) => {
    const result = await request<LoginResponse>('/api/auth/login', {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password }),
    })
    if (!result?.authenticated || !result.csrfToken || !result.expiresAt || !acceptSession(result.csrfToken, result.expiresAt)) {
      throw new Error('登录失败，请检查管理员账号和密码。')
    }
  }, [acceptSession])

  const signOut = useCallback(async () => {
    try {
      await request('/api/auth/logout', { method: 'POST' })
      expire()
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) expire()
      throw error
    }
  }, [expire])

  const value = useMemo(() => ({ status, signIn, signOut, reload }), [status, signIn, signOut, reload])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
