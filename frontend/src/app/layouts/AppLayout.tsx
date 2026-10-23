import { Button } from '@heroui/react'
import { Moon, Sun } from 'lucide-react'
import { Outlet } from 'react-router'
import { usePreferences } from '../../shared/stores/preferences'

export function AppLayout() {
  const { theme, setTheme } = usePreferences()
  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="flex items-center justify-between border-b border-separator px-6 py-4">
        <span className="text-lg font-semibold tracking-tight">115-media-hub</span>
        <Button variant="tertiary" onPress={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
          aria-label={theme === 'dark' ? '切换浅色主题' : '切换深色主题'}>
          {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
          主题
        </Button>
      </header>
      <main className="mx-auto max-w-5xl px-6 py-12"><Outlet /></main>
    </div>
  )
}
