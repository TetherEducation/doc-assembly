import { useAuthStore } from '@/stores/auth-store'

/**
 * OIDC configuration interface.
 */
export interface OIDCConfig {
  tokenEndpoint: string
  userinfoEndpoint: string
  endSessionEndpoint: string
  clientId: string
}

/**
 * Runtime OIDC config (set by AuthProvider after fetching from backend).
 */
let runtimeConfig: OIDCConfig | null = null

/**
 * Initialize OIDC config from runtime values (called by AuthProvider).
 */
export function initOIDCConfig(config: OIDCConfig): void {
  runtimeConfig = config
}

/**
 * Get current OIDC config.
 * Throws if not initialized (AuthProvider must run first).
 */
function getConfig(): OIDCConfig {
  if (!runtimeConfig) {
    throw new Error('OIDC config not initialized. AuthProvider must load config first.')
  }
  return runtimeConfig
}

/**
 * Token response from OIDC provider
 */
export interface TokenResponse {
  access_token: string
  refresh_token: string
  expires_in: number
  refresh_expires_in: number
  token_type: string
  id_token?: string
  scope?: string
}

/**
 * OIDC error response
 */
export interface OIDCError {
  error: string
  error_description?: string
}

/**
 * User info from OIDC provider
 */
export interface OIDCUserInfo {
  sub: string
  email?: string
  email_verified?: boolean
  preferred_username?: string
  given_name?: string
  family_name?: string
  name?: string
}

/**
 * Login with username and password using Direct Access Grant
 */
export async function loginWithCredentials(
  username: string,
  password: string
): Promise<TokenResponse> {
  const config = getConfig()

  const params = new URLSearchParams({
    grant_type: 'password',
    client_id: config.clientId,
    username,
    password,
    scope: 'openid profile email',
  })

  const response = await fetch(config.tokenEndpoint, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body: params.toString(),
    signal: AbortSignal.timeout(10_000),
  })

  if (!response.ok) {
    const error: OIDCError = await response.json()
    throw new Error(error.error_description || error.error || 'Login failed')
  }

  return response.json()
}

/**
 * Refresh access token using refresh token
 */
export async function refreshAccessToken(): Promise<TokenResponse> {
  const config = getConfig()

  if (!config.tokenEndpoint) {
    throw new Error('Token endpoint not configured (dummy auth mode)')
  }

  const { refreshToken } = useAuthStore.getState()

  if (!refreshToken) {
    throw new Error('No refresh token available')
  }

  const params = new URLSearchParams({
    grant_type: 'refresh_token',
    client_id: config.clientId,
    refresh_token: refreshToken,
  })

  const response = await fetch(config.tokenEndpoint, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body: params.toString(),
    signal: AbortSignal.timeout(10_000),
  })

  if (!response.ok) {
    const error: OIDCError = await response.json()
    throw new Error(error.error_description || error.error || 'Token refresh failed')
  }

  const tokens: TokenResponse = await response.json()

  // Update tokens in store
  useAuthStore.getState().setTokens(tokens.access_token, tokens.refresh_token, tokens.expires_in)

  return tokens
}

/**
 * Get user info from OIDC provider.
 *
 * Uses the stored token unless one is passed: a login reads the profile with
 * its new token before storing it (see completeLogin).
 */
export async function getUserInfo(
  accessToken?: string,
  fetchImpl: typeof fetch = fetch
): Promise<OIDCUserInfo> {
  const config = getConfig()
  const token = accessToken ?? useAuthStore.getState().token

  if (!token) {
    throw new Error('No access token available')
  }

  const response = await fetchImpl(config.userinfoEndpoint, {
    method: 'GET',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    signal: AbortSignal.timeout(10_000),
  })

  if (!response.ok) {
    throw new Error('Failed to get user info')
  }

  return response.json()
}

/**
 * Logout from OIDC provider and clear local auth state
 */
export async function logout(): Promise<void> {
  const { refreshToken, clearAuth } = useAuthStore.getState()

  // Clear local auth state first (don't show "session expired" toast on manual logout)
  clearAuth(false)

  if (refreshToken) {
    await endSession(refreshToken)
  }
}

/**
 * Ends the OIDC session behind a refresh token. Best effort: it never throws,
 * since the caller has already dropped the session locally. Used by logout and
 * by a login that is turned away (see completeLogin).
 */
export async function endSession(
  refreshToken: string,
  fetchImpl: typeof fetch = fetch
): Promise<void> {
  const config = getConfig()
  if (!refreshToken || !config.endSessionEndpoint) return

  try {
    const params = new URLSearchParams({
      client_id: config.clientId,
      refresh_token: refreshToken,
    })

    await fetchImpl(config.endSessionEndpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: params.toString(),
      signal: AbortSignal.timeout(10_000),
    })
  } catch (error) {
    console.warn('[Auth] Failed to end the OIDC session:', error)
  }
}

/**
 * Check if token needs refresh (expires within 60 seconds)
 */
export function shouldRefreshToken(): boolean {
  const { tokenExpiresAt } = useAuthStore.getState()
  if (!tokenExpiresAt) return false
  // Refresh if token expires within 60 seconds
  return Date.now() > tokenExpiresAt - 60000
}

/**
 * Setup automatic token refresh
 * Returns a cleanup function to stop the refresh interval
 */
export function setupTokenRefresh(): () => void {
  const refreshInterval = setInterval(async () => {
    const { token, refreshToken } = useAuthStore.getState()
    const shouldRefresh = shouldRefreshToken()

    // Only refresh if we have tokens and token needs refresh
    if (token && refreshToken && shouldRefresh) {
      try {
        await refreshAccessToken()
        console.log('[Auth] Token refreshed successfully')
      } catch (error) {
        console.error('[Auth] Failed to refresh token:', error)
        // Clear auth on refresh failure - user needs to login again
        useAuthStore.getState().clearAuth()
      }
    }
  }, 30000) // Check every 30 seconds

  return () => clearInterval(refreshInterval)
}

/**
 * Parse JWT token to get payload (without verification)
 */
export function parseJwtPayload(token: string): Record<string, unknown> | null {
  try {
    const parts = token.split('.')
    const base64Url = parts[1]
    if (!base64Url) return null

    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/')
    const jsonPayload = decodeURIComponent(
      atob(base64)
        .split('')
        .map((c) => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
        .join('')
    )
    return JSON.parse(jsonPayload)
  } catch {
    return null
  }
}
