// Components
export { AuthProvider } from './components/AuthProvider'
export { PermissionGuard } from './components/PermissionGuard'

// Hooks
export { usePermission, useCanAccessAdmin } from './hooks/usePermission'

// Login completion, for wrappers with their own sign-in flow
export { completeLogin } from './complete-login'
export type { CompleteLoginOptions, LoginOutcome, LoginStore } from './complete-login'

// API
export { fetchMyRoles, recordAccess } from './api/auth-api'

// Types and Rules
export * from './types'
