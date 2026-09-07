import { BrandIcon } from '@/components/common/BrandIcon'
import { APP_NAME } from '@/lib/brand'
import { LanguageSelector } from '@/components/common/LanguageSelector'
import { ThemeToggle } from '@/components/common/ThemeToggle'
import { Paginator } from '@/components/ui/paginator'
import { recordAccess } from '@/features/auth'
import { useMyTenants } from '@/features/tenants'
import { useWorkspaces } from '@/features/workspaces'
import { cn } from '@/lib/utils'
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip'
import { useAppContextStore, type TenantWithRole, type WorkspaceWithRole } from '@/stores/app-context-store'
import { useWorkspaceTransitionStore } from '@/stores/workspace-transition-store'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { AnimatePresence, motion } from 'framer-motion'
import { ArrowLeft, ArrowRight, Search } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

export const Route = createFileRoute('/select-tenant')({
  component: SelectTenantPage,
})

const containerVariants = {
  hidden: { opacity: 1 },
  visible: {
    opacity: 1,
    transition: {
      staggerChildren: 0.08,
    },
  },
  exit: {
    opacity: 1,
    transition: {
      staggerChildren: 0.05,
      staggerDirection: 1,
    },
  },
}

const itemVariants = {
  hidden: { opacity: 0, x: 0 },
  visible: {
    opacity: 1,
    x: 0,
    transition: { duration: 0.3 },
  },
  exit: {
    opacity: 0,
    x: -50,
    transition: { duration: 0.25, ease: 'easeIn' as const },
  },
}

const ITEMS_PER_PAGE = 5
const LIST_MIN_HEIGHT = 400 // Fixed height to prevent layout shifts

function LoadingDots() {
  const [dots, setDots] = useState('')

  useEffect(() => {
    const interval = setInterval(() => {
      setDots((prev) => (prev.length >= 3 ? '' : prev + '.'))
    }, 400)
    return () => clearInterval(interval)
  }, [])

  return <span className="inline-block w-6 text-left">{dots}</span>
}

function formatRelativeTime(isoDate: string | null | undefined): string {
  if (!isoDate) return 'Never'

  const date = new Date(isoDate)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMins = Math.floor(diffMs / 60000)
  const diffHours = Math.floor(diffMs / 3600000)
  const diffDays = Math.floor(diffMs / 86400000)
  const diffWeeks = Math.floor(diffDays / 7)

  if (diffMins < 1) return 'Just now'
  if (diffMins < 60) return `${diffMins} min${diffMins > 1 ? 's' : ''} ago`
  if (diffHours < 24) return `${diffHours} hour${diffHours > 1 ? 's' : ''} ago`
  if (diffDays < 7) return `${diffDays} day${diffDays > 1 ? 's' : ''} ago`
  return `${diffWeeks} week${diffWeeks > 1 ? 's' : ''} ago`
}

function SelectTenantPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { setCurrentTenant, setCurrentWorkspace, currentTenant } = useAppContextStore()
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedTenant, setSelectedTenant] = useState<TenantWithRole | null>(
    currentTenant as TenantWithRole | null
  )
  const [tenantPage, setTenantPage] = useState(1)
  const [workspacePage, setWorkspacePage] = useState(1)
  const [tenantTotalPages, setTenantTotalPages] = useState(1)
  const [workspaceTotalPages, setWorkspaceTotalPages] = useState(1)

  // Animation orchestration via global store (overlay persists across routes)
  const { startTransition, setPhase, phase: animationPhase } = useWorkspaceTransitionStore()
  const isAnimating = animationPhase !== 'idle' && animationPhase !== 'complete'

  // Effective query (only search when 3+ chars)
  const effectiveQuery = searchQuery.length >= 3 ? searchQuery : undefined

  // Fetch tenants with optional search
  const { data: tenantsData, isLoading: isLoadingTenants } = useMyTenants(tenantPage, ITEMS_PER_PAGE, effectiveQuery)

  // Fetch workspaces for selected tenant (only when a tenant is selected) with optional search
  const { data: workspacesData, isLoading: isLoadingWorkspaces } = useWorkspaces(
    selectedTenant?.id ?? null,
    workspacePage,
    ITEMS_PER_PAGE,
    selectedTenant ? effectiveQuery : undefined,
    'ACTIVE',
    true
  )

  // Pagination metadata
  const tenantPagination = tenantsData?.pagination
  const workspacePagination = workspacesData?.pagination

  // Minimum loading time state
  const [minLoadingComplete, setMinLoadingComplete] = useState(false)
  const [showMinCharsHint, setShowMinCharsHint] = useState(false)

  // Start minimum loading timer on mount and when context changes
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- Intentional reset
    setMinLoadingComplete(false)
    const timer = setTimeout(() => setMinLoadingComplete(true), 1000)
    return () => clearTimeout(timer)
  }, [selectedTenant])

  // Reset workspace page and total pages when tenant changes
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- Intentional reset
    setWorkspacePage(1)
    setWorkspaceTotalPages(1)
  }, [selectedTenant?.id])

  // Reset tenant page when search changes
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- Intentional reset
    setTenantPage(1)
  }, [searchQuery])

  // Show hint when 1-2 characters typed (debounced)
  useEffect(() => {
    if (searchQuery.length > 0 && searchQuery.length < 3) {
      const timer = setTimeout(() => setShowMinCharsHint(true), 300)
      return () => clearTimeout(timer)
    } else {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- Intentional state sync
      setShowMinCharsHint(false)
    }
  }, [searchQuery])

  // Update total pages when data arrives
  useEffect(() => {
    if (tenantPagination?.totalPages) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- Intentional sync with API
      setTenantTotalPages(tenantPagination.totalPages)
    }
  }, [tenantPagination?.totalPages])

  useEffect(() => {
    if (workspacePagination?.totalPages) {
      // eslint-disable-next-line react-hooks/set-state-in-effect -- Intentional sync with API
      setWorkspaceTotalPages(workspacePagination.totalPages)
    }
  }, [workspacePagination?.totalPages])

  // Combined loading conditions
  const showTenantLoading = !minLoadingComplete || isLoadingTenants || !tenantsData?.data
  const showWorkspaceLoading = !minLoadingComplete || isLoadingWorkspaces || (selectedTenant && !workspacesData?.data)

  const displayTenants: TenantWithRole[] = tenantsData?.data ?? []
  const displayWorkspaces: WorkspaceWithRole[] = workspacesData?.data ?? []

  const handleTenantSelect = (tenant: TenantWithRole) => {
    setSearchQuery('')
    setCurrentTenant(tenant as TenantWithRole)
    setSelectedTenant(tenant)
    recordAccess('TENANT', tenant.id).catch(() => {})
  }

  const handleWorkspaceSelect = async (workspace: WorkspaceWithRole, event: React.MouseEvent) => {
    // Capture full button dimensions
    const button = event.currentTarget as HTMLElement
    const rect = button.getBoundingClientRect()

    // Phase 1: Start transition via global store (moves to center)
    startTransition({ id: workspace.id, name: workspace.name }, rect)
    await new Promise(r => setTimeout(r, 600))

    // Phase 2: Fade borders while centered
    setPhase('fadeBorders')
    await new Promise(r => setTimeout(r, 400))

    // Phase 3: Fade out and navigate
    setPhase('fadeOut')
    setCurrentWorkspace(workspace as WorkspaceWithRole)
    recordAccess('WORKSPACE', workspace.id).catch(() => {})

    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- TanStack Router type limitation
    navigate({ to: '/workspace/$workspaceId', params: { workspaceId: workspace.id } as any })
  }

  const handleBack = () => {
    if (selectedTenant) {
      setSelectedTenant(null)
      setCurrentTenant(null)
    } else {
      navigate({ to: '/login' })
    }
  }

  return (
    <div className="relative flex min-h-screen flex-col items-center bg-background pt-32 lg:pt-40">
      {/* Logo pequeño en posición original con layoutId para animación */}
      <motion.div
        layoutId="app-logo"
        className="absolute left-6 top-8 flex items-center gap-3 md:left-12 lg:left-32"
      >
        <motion.div
          layoutId="app-logo-icon"
          className="flex h-6 w-6 items-center justify-center border-2 border-foreground"
        >
          <BrandIcon size={12} className="text-foreground" />
        </motion.div>
        <motion.span
          layoutId="app-logo-text"
          className="font-display text-sm font-bold uppercase tracking-tight text-foreground"
        >
          {APP_NAME}
        </motion.span>
      </motion.div>

      {/* Iconos arriba derecha con layoutId para animación */}
      <motion.div
        layoutId="app-controls"
        className="absolute right-6 top-8 flex items-center gap-1 md:right-12 lg:right-32"
      >
        <LanguageSelector />
        <ThemeToggle />
      </motion.div>

      {/* Main content - hides instantly when workspace is selected */}
      <motion.div
        animate={{
          opacity: isAnimating ? 0 : 1,
        }}
        transition={{ duration: 0 }}
        className="mx-auto grid w-full max-w-7xl grid-cols-1 items-start gap-16 px-6 py-24 md:px-12 lg:grid-cols-12 lg:gap-24 lg:px-32"
      >
        {/* Left column */}
        <motion.div layout className="lg:sticky lg:top-32 lg:col-span-4">
          {/* Indicador de organización seleccionada */}
          <AnimatePresence>
            {selectedTenant && (
              <motion.div
                key="org-indicator"
                layout
                initial={{ opacity: 0, y: -10 }}
                animate={{ 
                  opacity: 1, 
                  y: 0,
                  transition: { duration: 0.4, ease: 'easeOut' } 
                }}
                exit={{ 
                  opacity: 0, 
                  y: -10,
                  transition: { duration: 0.4, ease: 'easeIn' } 
                }}
                className="mb-6"
              >
                <span className="font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
                  Organization
                </span>
                <p className="mt-1 font-display text-lg font-medium tracking-tight text-foreground">
                  {selectedTenant.name}
                </p>
              </motion.div>
            )}
          </AnimatePresence>
          <motion.div layout transition={{ duration: 0.4, ease: 'easeOut' }}>
            <h1 className="mb-8 font-display text-5xl font-light leading-[1.05] tracking-tighter text-foreground md:text-6xl">
              {selectedTenant ? (
                <>
                  Select your
                  <br />
                  <span className="font-semibold">Workspace.</span>
                </>
              ) : (
                <>
                  {t('selectTenant.title', 'Select your')}
                  <br />
                  <span className="font-semibold">{t('selectTenant.subtitle', 'Organization.')}</span>
                </>
              )}
            </h1>
            <p className="mb-12 max-w-sm text-lg font-light leading-relaxed text-muted-foreground">
              {selectedTenant
                ? 'Choose a workspace to access document templates and assembly tools.'
                : t(
                    'selectTenant.description',
                    'Choose a tenant environment to access your document templates and assembly tools.'
                  )}
            </p>
          </motion.div>
          <button
            onClick={handleBack}
            className="group inline-flex items-center gap-2 font-mono text-sm text-muted-foreground transition-colors hover:text-foreground"
          >
            <ArrowLeft size={16} className="transition-transform group-hover:-translate-x-1" />
            <span>{selectedTenant ? 'Back to organizations' : t('selectTenant.back', 'Back to login')}</span>
          </button>
        </motion.div>

        {/* Right column */}
        <div className="flex flex-col justify-center lg:col-span-8">
          {/* Search */}
          <div className="group relative mb-8 w-full">
            <Search
              className="pointer-events-none absolute left-0 top-1/2 -translate-y-1/2 text-muted-foreground/50 transition-colors group-focus-within:text-foreground"
              size={20}
            />
            <input
              type="text"
              placeholder={selectedTenant ? 'Filter by workspace...' : t('selectTenant.filter', 'Filter by organization...')}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full rounded-none border-b border-border bg-transparent py-3 pl-10 pr-4 font-display text-xl text-foreground outline-none transition-colors placeholder:text-muted-foreground/30 focus-visible:border-foreground focus-visible:ring-0"
            />
            {showMinCharsHint && (
              <span className="absolute -bottom-6 left-0 text-xs text-muted-foreground/70">
                {t('selectTenant.minChars', 'Type at least 3 characters to search')}
              </span>
            )}
          </div>

          {/* List */}
          <div className="flex w-full flex-col justify-start" style={{ minHeight: `${LIST_MIN_HEIGHT}px` }}>
            <AnimatePresence mode="wait">
              {selectedTenant ? (
                // Workspaces list
                showWorkspaceLoading ? (
                  <motion.div
                    key="loading-workspaces"
                    initial={{ opacity: 0, y: 10 }}
                    animate={{ opacity: 1, y: 0, transition: { duration: 0.4, delay: 0.2 } }}
                    exit={{ opacity: 0, transition: { duration: 0.3 } }}
                    className="flex h-full items-start justify-center py-8 text-muted-foreground"
                  >
                    <span>Loading workspaces<LoadingDots /></span>
                  </motion.div>
                ) : displayWorkspaces.length === 0 ? (
                  <motion.div
                    key="empty-workspaces"
                    initial={{ opacity: 0, y: 10 }}
                    animate={{ opacity: 1, y: 0, transition: { duration: 0.4, delay: 0.2 } }}
                    exit={{ opacity: 0, transition: { duration: 0.3 } }}
                    className="flex h-full items-start justify-center py-8 text-muted-foreground"
                  >
                    <span>{t('common.noResults', 'No results found')}</span>
                  </motion.div>
                ) : (
                  <motion.div
                    key={`workspaces-page-${workspacePage}-${searchQuery.length >= 3 ? 'search' : 'list'}`}
                    variants={containerVariants}
                    initial="hidden"
                    animate="visible"
                    exit="exit"
                    className="flex h-full w-full flex-col justify-start"
                  >
                    {displayWorkspaces.map((ws: WorkspaceWithRole) => (
                      <motion.button
                        key={ws.id}
                        variants={itemVariants}
                        onClick={(e) => handleWorkspaceSelect(ws, e)}
                        disabled={!!isAnimating}
                        className={cn(
                          'group relative -mb-px flex w-full items-center justify-between rounded-sm border border-transparent border-b-border px-4 py-6 outline-none transition-all duration-200 hover:z-10 hover:border-foreground hover:bg-accent',
                          isAnimating && 'pointer-events-none'
                        )}
                      >
                        <div className="flex min-w-0 flex-col gap-1">
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <h3 className="max-w-[300px] truncate text-left font-display text-xl font-medium tracking-tight text-foreground md:max-w-[400px] md:text-2xl">
                                {ws.name}
                              </h3>
                            </TooltipTrigger>
                            <TooltipContent>{ws.name}</TooltipContent>
                          </Tooltip>
                          <span className="w-fit shrink-0 rounded-sm bg-muted px-1.5 py-0.5 font-mono text-[9px] font-bold uppercase tracking-widest text-muted-foreground">
                            {ws.code}
                          </span>
                        </div>
                        <div className="flex items-center gap-6 md:gap-8">
                          <span className="whitespace-nowrap font-mono text-[10px] text-muted-foreground transition-colors group-hover:text-foreground md:text-xs">
                            Last accessed: {formatRelativeTime(ws.lastAccessedAt)}
                          </span>
                          <ArrowRight
                            className="text-muted-foreground transition-all duration-300 group-hover:translate-x-1 group-hover:text-foreground"
                            size={24}
                          />
                        </div>
                      </motion.button>
                    ))}
                  </motion.div>
                )
              ) : // Tenants list
              showTenantLoading ? (
                <motion.div
                  key="loading-tenants"
                  initial={{ opacity: 0 }}
                  animate={{ opacity: 1 }}
                  exit={{ opacity: 0, transition: { duration: 0.3 } }}
                  className="flex h-full items-start justify-center py-8 text-muted-foreground"
                >
                  <span>Loading organizations<LoadingDots /></span>
                </motion.div>
              ) : displayTenants.length === 0 ? (
                <motion.div
                  key="empty-tenants"
                  initial={{ opacity: 0, y: 10 }}
                  animate={{ opacity: 1, y: 0, transition: { duration: 0.4, delay: 0.2 } }}
                  exit={{ opacity: 0, transition: { duration: 0.3 } }}
                  className="flex h-full items-start justify-center py-8 text-muted-foreground"
                >
                  <span>{t('common.noResults', 'No results found')}</span>
                </motion.div>
              ) : (
                <motion.div
                  key={`tenants-page-${tenantPage}-${searchQuery.length >= 3 ? 'search' : 'list'}`}
                  variants={containerVariants}
                  initial="hidden"
                  animate="visible"
                  exit="exit"
                  className="flex h-full w-full flex-col justify-start"
                >
                  {displayTenants.map((tenant: TenantWithRole) => (
                    <motion.button
                      key={tenant.id}
                      variants={itemVariants}
                      onClick={() => handleTenantSelect(tenant)}
                      className={cn(
                        'group relative -mb-px flex w-full items-center justify-between rounded-sm border border-transparent border-b-border px-4 py-6 outline-none transition-all duration-200 hover:z-10 hover:border-foreground hover:bg-accent'
                      )}
                    >
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <h3 className="max-w-[300px] truncate text-left font-display text-xl font-medium tracking-tight text-foreground transition-transform duration-300 group-hover:translate-x-2 md:max-w-[400px] md:text-2xl">
                            {tenant.name}
                          </h3>
                        </TooltipTrigger>
                        <TooltipContent>{tenant.name}</TooltipContent>
                      </Tooltip>
                      <div className="flex items-center gap-6 md:gap-8">
                        <span className="whitespace-nowrap font-mono text-[10px] text-muted-foreground transition-colors group-hover:text-foreground md:text-xs">
                          Last accessed: {formatRelativeTime(tenant.lastAccessedAt)}
                        </span>
                        <ArrowRight
                          className="text-muted-foreground transition-all duration-300 group-hover:translate-x-1 group-hover:text-foreground"
                          size={24}
                        />
                      </div>
                    </motion.button>
                  ))}
                </motion.div>
              )}
            </AnimatePresence>
          </div>

          {/* Pagination */}
          {!selectedTenant && (
            <Paginator
              page={tenantPage}
              totalPages={tenantTotalPages}
              onPageChange={setTenantPage}
              className="py-6"
            />
          )}
          {selectedTenant && (
            <Paginator
              page={workspacePage}
              totalPages={workspaceTotalPages}
              onPageChange={setWorkspacePage}
              className="py-6"
            />
          )}        </div>
      </motion.div>
    </div>
  )
}
