import { create } from 'zustand'
import { persist } from 'zustand/middleware'

/**
 * Theme options
 */
export type Theme = 'light' | 'dark' | 'system'

/**
 * Theme store state
 */
interface ThemeState {
  // State
  theme: Theme

  // Actions
  setTheme: (theme: Theme) => void

  // Computed
  getEffectiveTheme: () => 'light' | 'dark'
}

/**
 * Get system color scheme preference
 */
function getSystemTheme(): 'light' | 'dark' {
  if (typeof window === 'undefined') return 'light'
  return window.matchMedia('(prefers-color-scheme: dark)').matches
    ? 'dark'
    : 'light'
}

/**
 * Apply theme to document
 */
function applyTheme(theme: 'light' | 'dark') {
  if (typeof document === 'undefined') return

  const root = document.documentElement

  // Embedded public pages pin their theme (data-theme-lock) so the host
  // application keeps visual control; ignore store/system updates meanwhile.
  if (root.dataset.themeLock) return

  if (theme === 'dark') {
    root.classList.add('dark')
  } else {
    root.classList.remove('dark')
  }
}

/**
 * Theme store with persistence
 */
export const useThemeStore = create<ThemeState>()(
  persist(
    (set, get) => ({
      // Initial state
      theme: 'system',

      // Actions
      setTheme: (theme) => {
        set({ theme })

        // Apply the theme immediately
        const effectiveTheme = theme === 'system' ? getSystemTheme() : theme
        applyTheme(effectiveTheme)
      },

      // Computed
      getEffectiveTheme: () => {
        const { theme } = get()
        return theme === 'system' ? getSystemTheme() : theme
      },
    }),
    {
      name: 'doc-assembly-theme',
      onRehydrateStorage: () => (state) => {
        // Apply theme on rehydration
        if (state) {
          const effectiveTheme =
            state.theme === 'system' ? getSystemTheme() : state.theme
          applyTheme(effectiveTheme)
        }
      },
    }
  )
)

/**
 * Initialize theme system
 * Call this on app startup to set up system theme listener
 */
export function initializeTheme() {
  if (typeof window === 'undefined') return

  // Apply initial theme
  const { theme, getEffectiveTheme } = useThemeStore.getState()
  const effectiveTheme = theme === 'system' ? getSystemTheme() : theme
  applyTheme(effectiveTheme)

  // Listen for system theme changes
  const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')

  const handleChange = () => {
    const { theme } = useThemeStore.getState()
    if (theme === 'system') {
      const effectiveTheme = getEffectiveTheme()
      applyTheme(effectiveTheme)
    }
  }

  mediaQuery.addEventListener('change', handleChange)

  return () => {
    mediaQuery.removeEventListener('change', handleChange)
  }
}
