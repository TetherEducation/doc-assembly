import { createContext, useContext, useEffect } from 'react'
import { useThemeStore } from '@/stores/theme-store'

// TanStack Router JSON-parses search values: ?embedded=1 arrives as the
// number 1, ?embedded=true as boolean true, ?embedded=yes as a string.
type SearchValue = string | number | boolean | undefined

/**
 * Embedded mode: the public signing page is rendered inside a host
 * application's iframe (e.g. an admissions portal). The host owns all
 * chrome, so the page drops its own header/footer, pins the light theme
 * and reports terminal signing states to the host via postMessage.
 *
 * Activated with `?embedded=1` (or `embedded=true`) on /public/sign/:token.
 */
export function resolveEmbeddedMode(search: { embedded?: SearchValue }): boolean {
  const value = search.embedded
  return value === true || value === 1 || value === '1' || value === 'true'
}

export const EmbeddedModeContext = createContext(false)

export function useEmbeddedMode(): boolean {
  return useContext(EmbeddedModeContext)
}

export type ParentSigningEvent = 'embed.form.completed' | 'embed.form.exception'

/**
 * Notify the embedding host about a terminal signing state.
 *
 * The message shape ({ type, data }) intentionally matches the contract the
 * known hosts already listen for on their side of the iframe boundary.
 * The wildcard target origin is acceptable here: allowed embedders are
 * already restricted by frame-ancestors CSP, and the payload carries no
 * secrets — only the event name.
 */
export function notifyParentSigningEvent(event: ParentSigningEvent, data?: unknown): void {
  if (window.parent === window) return
  window.parent.postMessage({ type: event, data }, '*')
}

const THEME_LOCK_ATTR = 'themeLock'

/**
 * Pin the light theme while an embedded page is mounted, without touching
 * the visitor's persisted theme choice. `applyTheme` in the theme store
 * respects the lock, so system-theme changes cannot flip the page back.
 */
export function useEmbeddedThemeLock(embedded: boolean): void {
  useEffect(() => {
    if (!embedded) return

    const root = document.documentElement
    root.dataset[THEME_LOCK_ATTR] = 'light'
    root.classList.remove('dark')

    return () => {
      delete root.dataset[THEME_LOCK_ATTR]
      // Re-apply whatever the store says now that the lock is gone.
      const { theme, setTheme } = useThemeStore.getState()
      setTheme(theme)
    }
  }, [embedded])
}
