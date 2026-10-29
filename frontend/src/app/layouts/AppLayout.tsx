import { Alert, Avatar, Breadcrumbs, Button, Chip, Dropdown, Label, Tooltip } from '@heroui/react'
import {
  ChevronRight, Clapperboard, File, FolderOpen, LibraryBig,
  LayoutDashboard, ListTodo, Monitor, Moon, ScrollText, Settings, Sun, Upload,
} from 'lucide-react'
import { useState } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router'
import { useAuth } from '../../features/auth/auth-context'
import { ConnectionSidebarCard } from '../../features/connections/ConnectionSidebarCard'
import { useAppTheme } from '../../shared/theme/theme-context'

const navigation = [
  { label: '总览', icon: LayoutDashboard, path: '/' },
  { label: '网盘浏览', icon: FolderOpen, path: '/browse' },
  { label: '媒体库', icon: LibraryBig, path: '/libraries' },
  { label: 'STRM 管理', icon: File, path: '/strm' },
  { label: '任务中心', icon: ListTodo, path: '/tasks' },
  { label: '海报墙', icon: Clapperboard, path: '/media-wall' },
  { label: '刮削回传', icon: Upload, path: '/metadata' },
  { label: '运行日志', icon: ScrollText, path: '/logs' },
]

const themes = [
  { id: 'light', label: '浅色', icon: Sun },
  { id: 'dark', label: '深色', icon: Moon },
  { id: 'system', label: '跟随系统', icon: Monitor },
]

export function AppLayout() {
  const { theme, resolvedTheme, setTheme } = useAppTheme()
  const { signOut } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [logoutError, setLogoutError] = useState('')
  const [themeMenuOpen, setThemeMenuOpen] = useState(false)
  async function logout() {
    setLogoutError('')
    try { await signOut(); navigate('/login', { replace: true }) }
    catch { setLogoutError('暂时无法退出登录，请重试。') }
  }

  const active = navigation.find((item) => item.path === location.pathname
    || (item.path !== '/' && location.pathname.startsWith(`${item.path}/`)))
    ?? (location.pathname === '/connections' ? { label: '连接管理' } : { label: '页面不存在' })
  const themeLabel = themes.find((item) => item.id === theme)?.label ?? '跟随系统'
  const settingsSelected = location.pathname === '/connections'
  return (
    <div className="flex min-h-screen bg-background text-foreground">
      <aside className="sticky top-0 flex h-dvh w-[232px] shrink-0 flex-col bg-surface px-4 pb-4 pt-5 max-[720px]:w-[68px] max-[720px]:items-center max-[720px]:px-2">
        <div className="mb-6 shrink-0">
          <div className="flex items-center gap-2.5">
            <div className="flex size-[34px] shrink-0 items-center justify-center rounded-[10px] bg-accent text-accent-foreground">
              <Clapperboard aria-hidden size={20} strokeWidth={1.9} />
            </div>
            <div className="truncate text-lg font-bold tracking-tight max-[720px]:hidden">115 Media Hub</div>
          </div>
          <div className="mt-2 text-xs text-muted max-[720px]:hidden">媒体管理工作台</div>
        </div>

        <nav aria-label="主导航" className="flex min-h-0 w-full flex-1 flex-col gap-1 overflow-y-auto">
          {navigation.map(({ label, icon: Icon, path }) => {
            const selected = active.label === label
            return (
              <Button key={label} type="button" aria-current={selected ? 'page' : undefined} aria-label={label} onPress={() => navigate(path)}
                fullWidth className={`h-[42px] min-h-[42px] shrink-0 justify-start gap-3 rounded-lg px-3 text-sm font-normal max-[720px]:justify-center max-[720px]:px-0 ${selected ? 'bg-accent-soft text-accent-soft-foreground' : 'text-muted'}`} variant={selected ? 'secondary' : 'ghost'}>
                <Icon aria-hidden size={20} strokeWidth={1.8} />
                <span className="max-[720px]:hidden">{label}</span>
              </Button>
            )
          })}
        </nav>

        <div className="mt-4 flex w-full shrink-0 flex-col gap-5">
          <Button type="button" fullWidth className={`h-[42px] justify-start gap-3 rounded-lg px-3 font-normal max-[720px]:justify-center max-[720px]:px-0 ${settingsSelected ? 'bg-accent-soft text-accent-soft-foreground' : 'text-muted'}`} variant={settingsSelected ? 'secondary' : 'ghost'} aria-label="设置" aria-current={settingsSelected ? 'page' : undefined} onPress={() => navigate('/connections')}>
            <Settings aria-hidden size={20} strokeWidth={1.8} /><span className="max-[720px]:hidden">设置</span>
          </Button>
          <ConnectionSidebarCard />
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-16 shrink-0 items-center justify-between gap-3 bg-surface px-6 max-[520px]:px-3">
          <Breadcrumbs aria-label="工作台路径" separator={<ChevronRight aria-hidden size={14} />}>
            <Breadcrumbs.Item href="/" onClick={(event) => { event.preventDefault(); navigate('/') }}>工作台</Breadcrumbs.Item>
            <Breadcrumbs.Item>{active.label}</Breadcrumbs.Item>
          </Breadcrumbs>
          <div className="flex shrink-0 items-center gap-3 max-[520px]:gap-1">
            <span className="hidden sm:inline-flex"><Chip size="sm" color="accent" variant="soft">运行中任务 —</Chip></span>
            <Dropdown isOpen={themeMenuOpen} onOpenChange={setThemeMenuOpen}>
              <Button isIconOnly variant="ghost" size="sm" aria-label={`切换主题，当前${themeLabel}`}>
                {theme === 'system' ? <Monitor aria-hidden size={18} /> : resolvedTheme === 'dark' ? <Moon aria-hidden size={18} /> : <Sun aria-hidden size={18} />}
              </Button>
              <Dropdown.Popover placement="bottom end">
                <Dropdown.Menu aria-label="选择主题" selectionMode="single" selectedKeys={new Set([theme])} onAction={(key) => { setTheme(String(key)); setThemeMenuOpen(false) }}>
                  {themes.map(({ id, label, icon: Icon }) => <Dropdown.Item key={id} id={id} textValue={label}>
                    <Icon aria-hidden size={16} /><Label>{label}</Label><Dropdown.ItemIndicator />
                  </Dropdown.Item>)}
                </Dropdown.Menu>
              </Dropdown.Popover>
            </Dropdown>
            <Tooltip delay={250}>
              <Tooltip.Trigger>
                <Button isIconOnly variant="ghost" size="sm" onPress={() => void logout()} aria-label="退出登录">
                  <Avatar size="sm" color="accent"><Avatar.Fallback>A</Avatar.Fallback></Avatar>
                </Button>
              </Tooltip.Trigger>
              <Tooltip.Content placement="bottom">退出登录</Tooltip.Content>
            </Tooltip>
          </div>
        </header>
        {logoutError && <div className="mx-6 mt-3"><Alert status="danger"><Alert.Indicator /><Alert.Content><Alert.Description>{logoutError}</Alert.Description></Alert.Content></Alert></div>}
        <main className="min-h-[calc(100dvh-64px)] w-full min-w-0 flex-1 bg-background px-6 py-6 max-[520px]:px-3"><Outlet /></main>
      </div>
    </div>
  )
}
