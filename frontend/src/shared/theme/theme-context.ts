import { createContext, useContext } from 'react'
import type { useTheme } from '@heroui/react'

export const ThemeContext = createContext<ReturnType<typeof useTheme> | null>(null)

export function useAppTheme() {
  const value = useContext(ThemeContext)
  if (!value) throw new Error('useAppTheme must be used within AppProviders')
  return value
}
