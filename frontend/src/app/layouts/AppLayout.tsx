import { Button, Tooltip } from '@heroui/react'
import {
  Activity, Archive, Boxes, Clapperboard, FolderOpen,
  LayoutDashboard, ListTodo, Moon, Settings, Sun, Tags,
} from 'lucide-react'
import { useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router'
import { useAuth } from '../../features/auth/auth-context'
import { ConnectionSidebarCard } from '../../features/connections/ConnectionSidebarCard'
import { usePreferences } from '../../shared/stores/preferences'

const navigation = [
  { label: '总览', icon: LayoutDashboard, path: '/' },
  { label: '网盘浏览', icon: FolderOpen, path: '/browse' },
  { label: '媒体库', icon: Boxes, path: '/libraries' },
  { label: 'STRM 管理', icon: Archive, path: '/strm' },
  { label: '任务中心', icon: ListTodo, path: '/tasks' },
  { label: '海报墙', icon: Clapperboard, path: '/media-wall' },
  { label: '刮削回传', icon: Tags, path: '/metadata' },
  { label: '运行日志', icon: Activity, path: '/logs' },
]

export function AppLayout() {
  const { theme, setTheme } = usePreferences()
  const { signOut } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [logoutError, setLogoutError] = useState('')
  async function logout() {
    setLogoutError('')
    try { await signOut(); navigate('/login', { replace: true }) }
    catch { setLogoutError('暂时无法退出登录，请重试。') }
  }

  const active = navigation.find((item) => item.path === location.pathname
    || (item.path !== '/' && location.pathname.startsWith(`${item.path}/`)))
    ?? (location.pathname === '/connections' ? { label: '连接管理' } : navigation[0])
  const themeActionLabel = theme === 'dark' ? '切换浅色主题' : '切换深色主题'
  return (
    <div className="flex min-h-screen bg-background text-foreground">
      <aside className="sticky top-0 flex h-screen w-[248px] shrink-0 flex-col border-r border-separator bg-surface px-4 py-5 max-[720px]:w-[68px] max-[720px]:items-center max-[720px]:px-2">
        <div className="mb-8 flex items-center gap-3 px-2 max-[720px]:px-0">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-accent text-accent-foreground dark:bg-accent-soft dark:text-accent-soft-foreground">
            <Boxes size={22} strokeWidth={2.2} />
          </div>
          <div className="min-w-0 max-[720px]:hidden">
            <div className="truncate text-[15px] font-bold tracking-tight">115 Media Hub</div>
            <div className="mt-0.5 text-xs text-muted">媒体管理工作台</div>
          </div>
        </div>

        <nav aria-label="主导航" className="flex flex-1 flex-col gap-1">
          {navigation.map(({ label, icon: Icon, path }) => {
            const selected = active.label === label
            return (
              <button key={label} type="button" aria-current={selected ? 'page' : undefined} onClick={() => navigate(path)}
                className={`flex h-10 items-center gap-3 rounded-lg px-3 text-left text-sm font-medium max-[720px]:w-11 max-[720px]:justify-center max-[720px]:px-0 ${selected ? 'bg-accent-soft text-accent-soft-foreground' : 'text-muted transition-colors hover:bg-surface-secondary hover:text-foreground'}`}
                title={label}>
                <Icon size={18} strokeWidth={1.9} />
                <span className="max-[720px]:hidden">{label}</span>
              </button>
            )
          })}
        </nav>

        <div className="mt-4 border-t border-separator pt-4">
          <button type="button" className="flex h-10 w-full items-center gap-3 rounded-lg px-3 text-sm font-medium text-muted hover:bg-surface-secondary max-[720px]:w-11 max-[720px]:justify-center max-[720px]:px-0" title="设置">
            <Settings size={18} strokeWidth={1.9} /><span className="max-[720px]:hidden">设置</span>
          </button>
          <ConnectionSidebarCard />
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-[62px] shrink-0 items-center justify-between border-b border-separator bg-surface px-6 max-[520px]:px-3">
          <div className="flex min-w-0 items-center gap-2 text-sm">
            <span className="truncate text-muted">工作空间</span>
            <span className="text-muted">/</span>
            <span className="truncate font-semibold text-foreground">{active.label}</span>
          </div>
          <div className="flex shrink-0 items-center gap-3 max-[520px]:gap-1">
            <span className="hidden items-center gap-1.5 rounded-full bg-accent-soft px-2.5 py-1 text-xs font-medium text-accent-soft-foreground dark:bg-default dark:text-default-foreground sm:flex"><ListTodo size={14} />运行中任务 <span className="text-accent-soft-foreground dark:text-muted">—</span></span>
            <Tooltip delay={250}>
              <Tooltip.Trigger>
                <Button isIconOnly variant="tertiary" size="sm" onPress={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
                  aria-label={themeActionLabel}>
                  {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
                </Button>
              </Tooltip.Trigger>
              <Tooltip.Content placement="bottom">{themeActionLabel}</Tooltip.Content>
            </Tooltip>
            <Tooltip delay={250}>
              <Tooltip.Trigger>
                <Button isIconOnly variant="tertiary" size="sm" onPress={() => void logout()} aria-label="退出登录">
                  <span className="flex size-8 items-center justify-center rounded-full bg-accent-soft text-xs font-bold text-accent-soft-foreground">AD</span>
                </Button>
              </Tooltip.Trigger>
              <Tooltip.Content placement="bottom">退出登录</Tooltip.Content>
            </Tooltip>
          </div>
        </header>
        {logoutError && <p role="alert" className="px-6 pt-3 text-right text-sm text-danger">{logoutError}</p>}
        <main className="mx-auto max-w-5xl px-6 py-12"><Outlet /></main>
      </div>
    </div>
  )
}
