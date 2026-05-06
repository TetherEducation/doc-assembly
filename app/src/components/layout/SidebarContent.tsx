import { useState } from 'react'
import { Link, useLocation, useNavigate } from '@tanstack/react-router'
import { motion, AnimatePresence } from 'framer-motion'
import { LayoutGrid, FileText, FolderOpen, PenTool, Variable, Settings, Shield, LogOut, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useAuthStore } from '@/stores/auth-store'
import { useAppContextStore } from '@/stores/app-context-store'
import { useSandboxMode } from '@/stores/sandbox-mode-store'
import { logout } from '@/lib/oidc'
import { getInitials } from '@/lib/utils'
import { useTranslation } from 'react-i18next'
import { useWorkspaceTransitionStore } from '@/stores/workspace-transition-store'
import { SandboxIndicator } from '@/components/common/SandboxIndicator'
import { SandboxConfirmDialog } from '@/features/settings/components/SandboxConfirmDialog'
import { useQueryClient } from '@tanstack/react-query'

interface NavItem {
  label: string
  icon: typeof LayoutGrid
  href: string
  showInSandbox: boolean
  hideForSystem?: boolean
}

interface SidebarContentProps {
  /** Whether sidebar is expanded (shows text labels) */
  isExpanded: boolean
  /** Callback when navigation occurs (for closing mobile sidebar) */
  onNavigate?: () => void
  /** Whether to show animations (disable for mobile sheet) */
  showAnimations?: boolean
}

const navItemVariants = {
  hidden: { opacity: 0, x: -20 },
  visible: { opacity: 1, x: 0 },
}

const sandboxNavVariants = {
  initial: { opacity: 1, height: 'auto' },
  animate: { opacity: 1, height: 'auto' },
  exit: {
    opacity: 0,
    height: 0,
    marginTop: 0,
    marginBottom: 0,
    transition: { duration: 0.2, ease: [0.4, 0, 0.2, 1] },
  },
}

const footerItemVariants = {
  hidden: { opacity: 0, scale: 0 },
  visible: { opacity: 1, scale: 1 },
}

export function SidebarContent({
  isExpanded,
  onNavigate,
  showAnimations = true,
}: SidebarContentProps) {
  const { t } = useTranslation()
  const location = useLocation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { userProfile } = useAuthStore()
  const { currentWorkspace, clearContext, isSystemContext } = useAppContextStore()
  const { isSandboxActive, disableSandbox } = useSandboxMode()
  const { phase: transitionPhase } = useWorkspaceTransitionStore()
  const [showExitDialog, setShowExitDialog] = useState(false)

  // Hide workspace name while transition animation is active
  const showWorkspaceName = transitionPhase === 'idle' || transitionPhase === 'complete'

  const workspaceId = currentWorkspace?.id || ''
  const isSystem = isSystemContext()

  const allNavItems: NavItem[] = [
    {
      label: t('nav.dashboard'),
      icon: LayoutGrid,
      href: `/workspace/${workspaceId}`,
      showInSandbox: false,
    },
    ...(!isSystem
      ? [
          {
            label: t('nav.templates'),
            icon: FileText,
            href: `/workspace/${workspaceId}/templates`,
            showInSandbox: true,
          },
          {
            label: t('nav.documents'),
            icon: FolderOpen,
            href: `/workspace/${workspaceId}/documents`,
            showInSandbox: true,
          },
          {
            label: t('nav.signing'),
            icon: PenTool,
            href: `/workspace/${workspaceId}/signing`,
            showInSandbox: false,
          },
        ]
      : []),
    {
      label: t('nav.variables'),
      icon: Variable,
      href: `/workspace/${workspaceId}/variables`,
      showInSandbox: true,
    },
    {
      label: t('nav.settings'),
      icon: Settings,
      href: `/workspace/${workspaceId}/settings`,
      showInSandbox: false,
      hideForSystem: true,
    },
    // Administration - only visible in SYSTEM workspace
    ...(isSystemContext()
      ? [
          {
            label: t('nav.administration', 'Administration'),
            icon: Shield,
            href: `/workspace/${workspaceId}/administration`,
            showInSandbox: false,
          },
        ]
      : []),
  ]

  // Filter nav items based on sandbox mode and workspace type
  const visibleNavItems = allNavItems.filter((item) => {
    if (isSandboxActive && !item.showInSandbox) return false
    if (isSystem && item.hideForSystem) return false
    return true
  })

  const displayName = userProfile
    ? `${userProfile.firstName || ''} ${userProfile.lastName || ''}`.trim() ||
      userProfile.username ||
      'User'
    : 'User'

  const email = userProfile?.email || 'user@example.com'
  const initials = getInitials(displayName)

  const handleLogout = async () => {
    onNavigate?.()
    clearContext()
    await logout()
    navigate({ to: '/login' })
  }

  const handleNavClick = () => {
    onNavigate?.()
  }

  const handleExitSandbox = () => {
    disableSandbox()
    // Invalidate queries to refetch without sandbox header
    queryClient.invalidateQueries({ queryKey: ['templates'] })
    queryClient.invalidateQueries({ queryKey: ['folders'] })
    setShowExitDialog(false)

    // Redirect to section root if on a detail/editor/folder route (sandbox data won't exist in production)
    const templatesBase = `/workspace/${workspaceId}/templates`
    const documentsBase = `/workspace/${workspaceId}/documents`
    const searchParams = location.search as Record<string, unknown>
    const hasFolder = 'folderId' in searchParams

    const isOnTemplatesDetail = location.pathname.startsWith(templatesBase) && location.pathname !== templatesBase
    const isOnDocumentsDetail = location.pathname.startsWith(documentsBase) && location.pathname !== documentsBase
    const isOnTemplates = location.pathname.startsWith(templatesBase)
    const isOnDocuments = location.pathname.startsWith(documentsBase)

    if (isOnTemplatesDetail || (isOnTemplates && hasFolder)) {
      navigate({ to: templatesBase })
    } else if (isOnDocumentsDetail || (isOnDocuments && hasFolder)) {
      navigate({ to: documentsBase })
    }
  }

  const isActive = (href: string) => {
    // Exact match for dashboard (index route)
    if (href === `/workspace/${workspaceId}`) {
      return location.pathname === href
    }
    // Starts with for other routes
    return location.pathname.startsWith(href)
  }

  // Animation delay - 0 for mobile (no delay), standard for desktop
  const animationDelay = showAnimations ? 0.8 : 0

  return (
    <>
      <ScrollArea className={cn('flex-1 py-6', isExpanded ? 'px-4' : 'px-2')}>
        {/* Current Workspace */}
        {currentWorkspace && (
          <div className="relative mb-8 px-1">
            {/* Avatar - visible solo cuando colapsado */}
            <motion.div
              initial={false}
              animate={{
                opacity: isExpanded || !showWorkspaceName ? 0 : 1,
                scale: isExpanded ? 0.8 : 1,
              }}
              transition={{ duration: 0.2, ease: [0.4, 0, 0.2, 1] }}
              className="absolute inset-0 flex items-center"
              style={{ pointerEvents: isExpanded || !showWorkspaceName ? 'none' : 'auto' }}
            >
              <Tooltip>
                <TooltipTrigger asChild>
                  <Avatar className="h-10 w-10 cursor-default">
                    <AvatarFallback className="bg-primary/10 text-sm font-bold">
                      {getInitials(currentWorkspace.name)}
                    </AvatarFallback>
                  </Avatar>
                </TooltipTrigger>
                <TooltipContent side="right" sideOffset={8}>
                  <div className="text-xs text-muted-foreground">
                    {t('workspace.current')}
                  </div>
                  <div className="font-medium">{currentWorkspace.name}</div>
                  {isSandboxActive && (
                    <div className="mt-1">
                      <SandboxIndicator variant="badge" />
                    </div>
                  )}
                </TooltipContent>
              </Tooltip>
            </motion.div>

            {/* Texto - visible solo cuando expandido */}
            <motion.div
              initial={false}
              animate={{
                opacity: isExpanded ? 1 : 0,
                x: isExpanded ? 0 : -10,
              }}
              transition={{ duration: 0.2, ease: [0.4, 0, 0.2, 1] }}
              className="flex h-10 items-center"
              style={{ pointerEvents: isExpanded ? 'auto' : 'none' }}
            >
              <div style={{ opacity: showWorkspaceName ? 1 : 0 }}>
                <label className="block text-[10px] font-mono uppercase tracking-widest text-muted-foreground">
                  {t('workspace.current')}
                </label>
                <div className="truncate font-display text-lg font-medium">
                  {currentWorkspace.name}
                </div>
              </div>
            </motion.div>

            {/* Sandbox indicator with exit button - visible when expanded and sandbox active */}
            <AnimatePresence>
              {isSandboxActive && isExpanded && (
                <motion.div
                  initial={{ opacity: 0, y: -5, height: 0 }}
                  animate={{ opacity: 1, y: 0, height: 'auto' }}
                  exit={{ opacity: 0, y: -5, height: 0 }}
                  transition={{ duration: 0.2 }}
                  className="mt-2 flex items-center justify-between"
                >
                  <SandboxIndicator variant="label" />
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        onClick={() => setShowExitDialog(true)}
                        className="flex h-6 w-6 items-center justify-center rounded text-sandbox transition-colors hover:bg-sandbox-muted"
                      >
                        <X size={14} strokeWidth={2} />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent side="right" sideOffset={8}>
                      {t('sandbox.exit', 'Exit Sandbox')}
                    </TooltipContent>
                  </Tooltip>
                </motion.div>
              )}
            </AnimatePresence>

            {/* Sandbox exit button - visible when collapsed */}
            <AnimatePresence>
              {isSandboxActive && !isExpanded && (
                <motion.div
                  initial={{ opacity: 0, scale: 0.8 }}
                  animate={{ opacity: 1, scale: 1 }}
                  exit={{ opacity: 0, scale: 0.8 }}
                  transition={{ duration: 0.2 }}
                  className="mt-2 flex justify-center"
                >
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <button
                        onClick={() => setShowExitDialog(true)}
                        className="flex h-8 w-8 items-center justify-center rounded border border-sandbox-border bg-sandbox-muted text-sandbox transition-colors hover:bg-sandbox/20"
                      >
                        <X size={14} strokeWidth={2} />
                      </button>
                    </TooltipTrigger>
                    <TooltipContent side="right" sideOffset={8}>
                      <div className="text-xs text-muted-foreground">
                        {t('sandbox.label', 'Sandbox')}
                      </div>
                      <div className="font-medium">{t('sandbox.exit', 'Exit Sandbox')}</div>
                    </TooltipContent>
                  </Tooltip>
                </motion.div>
              )}
            </AnimatePresence>
          </div>
        )}

        {/* Navigation */}
        <nav className="space-y-1">
          <AnimatePresence mode="popLayout" initial={false}>
            {visibleNavItems.map((item, index) => {
              const active = isActive(item.href)
              return (
                <motion.div
                  key={item.href}
                  variants={sandboxNavVariants}
                  initial="initial"
                  animate="animate"
                  exit="exit"
                >
                  <motion.div
                    variants={showAnimations ? navItemVariants : undefined}
                    initial={showAnimations ? 'hidden' : false}
                    animate={showAnimations ? 'visible' : false}
                    transition={{ duration: 0.3, delay: animationDelay + index * 0.08 }}
                  >
                    <Link
                      to={item.href}
                      onClick={handleNavClick}
                      className={cn(
                        'group flex w-full items-center gap-4 rounded-md px-3 py-3 text-sm font-medium transition-colors',
                        active
                          ? 'bg-sidebar-accent text-sidebar-accent-foreground'
                          : 'text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground'
                      )}
                    >
                      <item.icon
                        size={20}
                        strokeWidth={1.5}
                        className={cn(
                          'shrink-0',
                          active
                            ? 'text-sidebar-accent-foreground'
                            : 'text-muted-foreground group-hover:text-sidebar-accent-foreground'
                        )}
                      />
                      <motion.span
                        initial={false}
                        animate={{
                          opacity: isExpanded ? 1 : 0,
                          width: isExpanded ? 'auto' : 0,
                        }}
                        transition={{ duration: 0.15, ease: [0.4, 0, 0.2, 1] }}
                        className="overflow-hidden whitespace-nowrap font-mono"
                      >
                        {item.label}
                      </motion.span>
                    </Link>
                  </motion.div>
                </motion.div>
              )
            })}
          </AnimatePresence>
        </nav>
      </ScrollArea>

      {/* Footer */}
      <div className={cn('border-t py-4', isExpanded ? 'px-4' : 'px-2')}>
        {/* User profile */}
        <div className="flex items-center gap-3 rounded-md px-2 py-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <Avatar className="h-8 w-8 shrink-0 cursor-default">
                <AvatarFallback className="text-xs font-bold">
                  {initials}
                </AvatarFallback>
              </Avatar>
            </TooltipTrigger>
            {!isExpanded && (
              <TooltipContent side="right" sideOffset={8}>
                <div className="font-medium">{email}</div>
                <div className="text-xs text-muted-foreground">
                  {currentWorkspace?.role || 'Member'}
                </div>
              </TooltipContent>
            )}
          </Tooltip>

          <motion.div
            initial={false}
            animate={{
              opacity: isExpanded ? 1 : 0,
              width: isExpanded ? 'auto' : 0,
            }}
            transition={{ duration: 0.15, ease: [0.4, 0, 0.2, 1] }}
            className="min-w-0 overflow-hidden"
          >
            <div className="truncate text-xs font-semibold whitespace-nowrap">
              {email}
            </div>
            <div className="text-[10px] uppercase text-muted-foreground whitespace-nowrap">
              {currentWorkspace?.role || 'Member'}
            </div>
          </motion.div>
        </div>

        {/* Logout button */}
        <motion.div
          variants={showAnimations ? footerItemVariants : undefined}
          initial={showAnimations ? 'hidden' : false}
          animate={showAnimations ? 'visible' : false}
          transition={{ duration: 0.3, delay: animationDelay }}
        >
          <button
            onClick={handleLogout}
            className="group flex w-full items-center gap-4 rounded-md px-3 py-3 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
          >
            <LogOut
              size={20}
              strokeWidth={1.5}
              className="shrink-0 transition-transform group-hover:-translate-x-1"
            />
            <motion.span
              initial={false}
              animate={{
                opacity: isExpanded ? 1 : 0,
                width: isExpanded ? 'auto' : 0,
              }}
              transition={{ duration: 0.15, ease: [0.4, 0, 0.2, 1] }}
              className="overflow-hidden whitespace-nowrap font-mono"
            >
              {t('nav.logout')}
            </motion.span>
          </button>
        </motion.div>
      </div>

      {/* Exit Sandbox Confirmation Dialog */}
      <SandboxConfirmDialog
        open={showExitDialog}
        onOpenChange={setShowExitDialog}
        action="disable"
        onConfirm={handleExitSandbox}
      />
    </>
  )
}
