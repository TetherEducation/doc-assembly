import { createRootRoute, Outlet, useNavigate, useLocation } from '@tanstack/react-router'
import { AnimatePresence, LayoutGroup } from 'framer-motion'
import { useEffect, useMemo } from 'react'
import { useAuthStore } from '@/stores/auth-store'
import { WorkspaceTransitionOverlay } from '@/components/layout/WorkspaceTransitionOverlay'
import { isPublicPathname, stripBasePath } from '@/lib/public-route-guard'

export const Route = createRootRoute({
  component: RootLayout,
})

function RootLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated())
  const isAuthLoading = useAuthStore((state) => state.isAuthLoading)
  const appPathname = stripBasePath(location.pathname, import.meta.env.VITE_BASE_PATH || '')

  // Calcular key que agrupa rutas del mismo layout
  // Para rutas /workspace/xxx/* usar solo /workspace/xxx
  // Para otras rutas usar el pathname completo
  const layoutKey = useMemo(() => {
    const match = appPathname.match(/^\/workspace\/[^/]+/)
    return match ? match[0] : appPathname
  }, [appPathname])

  useEffect(() => {
    // Skip check while auth is loading
    if (isAuthLoading) return

    // Check if current route is public
    const isPublicRoute = isPublicPathname(location.pathname)

    // If not authenticated and not on public route, redirect to login
    if (!isAuthenticated && !isPublicRoute) {
      console.log('[Auth Guard] Not authenticated, redirecting to login')
      navigate({ to: '/login', replace: true })
    }

    // If authenticated and on login page, redirect away (handles dummy auth auto-login)
    if (isAuthenticated && appPathname === '/login') {
      navigate({ to: '/select-tenant', replace: true })
    }
  }, [appPathname, isAuthenticated, isAuthLoading, location.pathname, navigate])

  return (
    <LayoutGroup>
      <div className="min-h-screen bg-background text-foreground">
        <AnimatePresence mode="wait" initial={false}>
          <div key={layoutKey}>
            <Outlet />
          </div>
        </AnimatePresence>

        {/* Overlay que persiste entre rutas para animaciones de transición */}
        <WorkspaceTransitionOverlay />
      </div>
    </LayoutGroup>
  )
}
