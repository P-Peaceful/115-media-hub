import { useInsertionEffect, useState, type ReactNode } from 'react'
import { useTheme } from '@heroui/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ThemeContext } from '../../shared/theme/theme-context'
import { AuthProvider } from '../../features/auth/AuthProvider'

export function AppProviders({ children }: { children: ReactNode }) {
  const [queryClient] = useState(() => new QueryClient({
    defaultOptions: { queries: { staleTime: 30_000, retry: 1 }, mutations: { retry: false } },
  }))
  const theme = useTheme('system')
  useInsertionEffect(() => {
    // Install before HeroUI applies the new theme in its layout effect. Otherwise
    // component color transitions briefly mix the old palette with the new canvas.
    const style = document.createElement('style')
    style.dataset.themeTransitionGuard = ''
    style.textContent = '*, *::before, *::after { transition: none !important; }'
    document.head.appendChild(style)
    const frame = requestAnimationFrame(() => {
      // Resolve the new palette with transitions disabled before restoring them.
      void window.getComputedStyle(document.body).opacity
      style.remove()
    })
    return () => {
      cancelAnimationFrame(frame)
      style.remove()
    }
  }, [theme.resolvedTheme])
  return <ThemeContext value={theme}><QueryClientProvider client={queryClient}><AuthProvider>{children}</AuthProvider></QueryClientProvider></ThemeContext>
}
