import { createContext, useContext } from 'react'

export type AuthStatus = 'loading' | 'authenticated' | 'unauthenticated' | 'error'
export type AuthContextValue = { status: AuthStatus; signIn: (username: string, password: string) => Promise<void>; signOut: () => Promise<void>; reload: () => Promise<void> }
export const AuthContext = createContext<AuthContextValue | null>(null)

export function useAuth() {
  const value = useContext(AuthContext)
  if (!value) throw new Error('useAuth must be used within AuthProvider')
  return value
}
