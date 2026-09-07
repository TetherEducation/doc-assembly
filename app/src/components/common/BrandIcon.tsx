import { Box } from 'lucide-react'

import { cn } from '@/lib/utils'

/**
 * The square brand mark shown next to the product name.
 *
 * Wrappers embed this SPA under their own identity, so the mark is a build
 * input rather than a hardcoded icon — the same approach as VITE_APP_NAME:
 *
 *   VITE_APP_ICON_SRC="/brand/icon.svg"
 *   VITE_APP_ICON_SRC_DARK="/brand/icon-white.svg"   # optional
 *
 * With neither set it renders the lucide cube the app has always used, so
 * tools-doc-assembly is unaffected. The dark variant is optional and falls back
 * to the light one; both are rendered and toggled with Tailwind's `dark:`
 * classes so switching themes needs no JavaScript and cannot flash the wrong
 * asset on first paint.
 */
interface BrandIconProps {
  /** Rendered size in pixels. Matches the lucide `size` prop it replaces. */
  size?: number
  className?: string
}

const ICON_SRC = import.meta.env.VITE_APP_ICON_SRC as string | undefined
const ICON_SRC_DARK =
  (import.meta.env.VITE_APP_ICON_SRC_DARK as string | undefined) || ICON_SRC

export function BrandIcon({ size = 16, className }: BrandIconProps) {
  if (!ICON_SRC) {
    return <Box size={size} fill="currentColor" className={className} />
  }

  // alt="" — the product name sits beside it, so announcing the mark as well
  // would make screen readers say it twice.
  return (
    <>
      <img
        src={ICON_SRC}
        alt=""
        width={size}
        height={size}
        className={cn('block dark:hidden', className)}
        style={{ width: size, height: size }}
      />
      <img
        src={ICON_SRC_DARK}
        alt=""
        width={size}
        height={size}
        className={cn('hidden dark:block', className)}
        style={{ width: size, height: size }}
      />
    </>
  )
}

/** True when a wrapper supplied its own mark. */
export const HAS_BRAND_ICON = Boolean(ICON_SRC)
