import { create } from 'zustand'
import { persist } from 'zustand/middleware'

type Theme = 'light' | 'dark'
interface Preferences {
  theme: Theme
  setTheme: (theme: Theme) => void
}

export const usePreferences = create<Preferences>()(
  persist(
    (set) => ({ theme: 'light', setTheme: (theme) => set({ theme }) }),
    { name: '115-media-hub-preferences', version: 1, partialize: ({ theme }) => ({ theme }) },
  ),
)
