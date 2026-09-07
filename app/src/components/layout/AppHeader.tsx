import { BrandIcon } from '@/components/common/BrandIcon'
import { APP_NAME } from '@/lib/brand'
import { motion } from 'framer-motion'
import { Menu } from 'lucide-react'
import { cn } from '@/lib/utils'
import { ThemeToggle } from '@/components/common/ThemeToggle'
import { LanguageSelector } from '@/components/common/LanguageSelector'
import { ContextBreadcrumb } from '@/components/common/ContextBreadcrumb'
import { Button } from '@/components/ui/button'
import { useSandboxMode } from '@/stores/sandbox-mode-store'

interface AppHeaderProps {
  variant?: 'minimal' | 'full'
  className?: string
  showMobileMenu?: boolean
  onMobileMenuToggle?: () => void
}

export function AppHeader({
  variant = 'minimal',
  className,
  showMobileMenu = false,
  onMobileMenuToggle
}: AppHeaderProps) {
  const isMinimal = variant === 'minimal'
  const { isSandboxActive } = useSandboxMode()

  return (
    <motion.header
      className={cn(
        'fixed left-0 right-0 top-0 z-50 flex h-16 items-center justify-between bg-background',
        isMinimal ? 'px-6 md:px-12 lg:px-32' : 'px-4 sm:px-6',
        className
      )}
    >
      {/* Logo and context breadcrumb */}
      <div className="flex items-center gap-3">
        {/* Mobile menu button - left side for standard UX */}
        {showMobileMenu && (
          <Button
            variant="ghost"
            size="icon"
            onClick={onMobileMenuToggle}
            className="shrink-0 lg:hidden"
            aria-label="Toggle navigation menu"
          >
            <Menu className="h-5 w-5" />
          </Button>
        )}

        {/* Logo grande con layoutId para animación */}
        <motion.div
          layoutId="app-logo"
          className="flex items-center gap-3"
        >
          <motion.div
            layoutId="app-logo-icon"
            className={cn(
              'flex h-8 w-8 items-center justify-center border-2 transition-colors',
              isSandboxActive ? 'border-sandbox' : 'border-foreground'
            )}
          >
            <BrandIcon
              size={16}
              className={cn(
                'transition-colors',
                isSandboxActive ? 'text-sandbox' : 'text-foreground'
              )}
            />
          </motion.div>
          <motion.span
            layoutId="app-logo-text"
            className={cn(
              'font-display text-lg font-bold uppercase tracking-tight transition-colors',
              isSandboxActive ? 'text-sandbox' : 'text-foreground'
            )}
          >
            {APP_NAME}
          </motion.span>
        </motion.div>

        {/* Context breadcrumb - only in full variant, hidden on mobile */}
        {!isMinimal && (
          <div className="hidden sm:block">
            <ContextBreadcrumb />
          </div>
        )}
      </div>

      {/* Controles de idioma y tema con layoutId para animación */}
      <motion.div
        layoutId="app-controls"
        className="flex shrink-0 items-center gap-1"
      >
        <LanguageSelector />
        <ThemeToggle />
      </motion.div>

      {/* Línea del borde - slide desde izquierda */}
      <motion.div
        className="absolute bottom-0 left-0 right-0 h-px bg-border origin-left"
        initial={{ scaleX: 0 }}
        animate={{ scaleX: isMinimal ? 0 : 1 }}
        transition={{ duration: 0.5, delay: 0.3 }}
      />
    </motion.header>
  )
}
