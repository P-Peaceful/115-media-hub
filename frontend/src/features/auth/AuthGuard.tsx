import { Button, Spinner } from '@heroui/react'
import { Navigate, Outlet, useLocation } from 'react-router'
import { useAuth } from './auth-context'

export function AuthGuard() {
  const { status, reload } = useAuth()
  const location = useLocation()
  if (status === 'loading') return <main className="flex min-h-screen items-center justify-center gap-3" aria-live="polite"><Spinner aria-label="正在验证管理会话" />正在验证管理会话…</main>
  if (status === 'error') return <main className="grid min-h-screen place-items-center p-6"><section className="space-y-4 text-center"><h1 className="text-xl font-semibold">无法验证登录状态</h1><p>请检查服务连接后重试。</p><Button variant="primary" className="rounded-lg" onPress={() => void reload()}>重试</Button></section></main>
  if (status !== 'authenticated') return <Navigate to="/login" replace state={{ from: `${location.pathname}${location.search}${location.hash}` }} />
  return <Outlet />
}
