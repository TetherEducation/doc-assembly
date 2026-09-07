import { APP_NAME } from '@/lib/brand'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import { routeTree } from './routeTree.gen'
import { AuthProvider } from '@/features/auth/components/AuthProvider'
import { Toaster } from '@/components/ui/toaster'
import { TooltipProvider } from '@/components/ui/tooltip'
import './index.css'
import './brand-overrides.css'

declare const __BUILD_TIMESTAMP__: string
console.log(`[${APP_NAME}] Build: ${__BUILD_TIMESTAMP__}`)

// Create a new router instance
// @ts-expect-error - TanStack Router requires strictNullChecks but we have it disabled
const router = createRouter({
  routeTree,
  basepath: import.meta.env.VITE_BASE_PATH || '/',
})

// Register the router instance for type safety
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

// Create a client for React Query
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 60 * 5, // 5 minutes
      retry: 1,
    },
  },
})

// Get root element
const rootElement = document.getElementById('root')

if (!rootElement) {
  throw new Error('Root element not found')
}

// Create root and render
createRoot(rootElement).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <TooltipProvider delayDuration={300}>
        <AuthProvider>
          <RouterProvider router={router} />
          <Toaster />
        </AuthProvider>
      </TooltipProvider>
    </QueryClientProvider>
  </StrictMode>
)
