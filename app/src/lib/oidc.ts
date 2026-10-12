import { toAbsoluteAppUrl } from '@/lib/app-url'
import { useAuthStore } from '@/stores/auth-store'

/**
 * OIDC configuration interface.
 */
export interface OIDCConfig {
  /**
   * Where the redirect login sends the browser. Optional because backends that
   * predate the redirect login don't send it; that flow refuses to start
   * without it rather than guess the URL.
   */
  authorizationEndpoint?: string
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

/*
 * Redirect login (authorization code + PKCE).
 *
 * The browser leaves for the provider's login page — or, with an IdP hint,
 * straight for a federated provider such as Google — and comes back to this
 * app's login route with a one-time code, which is exchanged for tokens here.
 * What the return leg needs (the PKCE verifier, the state proving this tab
 * started the attempt, the exact redirect URI) waits in sessionStorage under a
 * single key in between.
 */

const REDIRECT_LOGIN_STORAGE_KEY = 'doc-assembly:oidc-redirect-login'

/** How long a started redirect login stays redeemable. */
const REDIRECT_LOGIN_MAX_AGE_MS = 10 * 60 * 1000

interface PendingRedirectLogin {
  state: string
  verifier: string
  redirectUri: string
  createdAt: number
}

export interface StartRedirectLoginOptions {
  /**
   * Skip the provider's own login page and go straight to this identity
   * provider (Keycloak's `kc_idp_hint`, e.g. `Google`).
   */
  idpHint?: string
  /**
   * Where the provider sends the browser back; it must be one the client
   * allows. Defaults to this app's login route under `VITE_BASE_PATH`
   * (e.g. `https://host/team/login`).
   */
  redirectUri?: string
}

/**
 * Start a redirect login: remember the PKCE verifier and state for the return
 * leg, then send the browser to the provider's authorization endpoint.
 * Resolves once the navigation has been requested.
 */
export async function startRedirectLogin(
  options: StartRedirectLoginOptions = {}
): Promise<void> {
  const config = getConfig()

  if (!config.authorizationEndpoint) {
    throw new Error(
      'Redirect login is unavailable: the server did not provide an authorization endpoint.'
    )
  }

  const redirectUri = options.redirectUri || toAbsoluteAppUrl('/login')
  const verifier = randomBase64Url(32)
  const challenge = await sha256Base64Url(verifier)
  const state = randomBase64Url(32)

  const url = new URL(config.authorizationEndpoint)
  url.searchParams.set('response_type', 'code')
  url.searchParams.set('client_id', config.clientId)
  url.searchParams.set('redirect_uri', redirectUri)
  url.searchParams.set('scope', 'openid profile email')
  url.searchParams.set('state', state)
  url.searchParams.set('code_challenge', challenge)
  url.searchParams.set('code_challenge_method', 'S256')
  if (options.idpHint) {
    url.searchParams.set('kc_idp_hint', options.idpHint)
  }

  const pending: PendingRedirectLogin = {
    state,
    verifier,
    redirectUri,
    createdAt: Date.now(),
  }
  window.sessionStorage.setItem(REDIRECT_LOGIN_STORAGE_KEY, JSON.stringify(pending))

  window.location.assign(url.toString())
}

/**
 * Finish a redirect login from the query string the provider came back with.
 *
 * Returns null when there is nothing for this tab to redeem: no `code` (and no
 * `error`) in the query, or a `state` that doesn't match a redirect login this
 * tab started in the last ten minutes. The second case is deliberate — Keycloak
 * also lands on the login route with a code after flows this app never started
 * (e.g. a password-reset link), and that code must be left alone: not redeemed,
 * and not reported as an error.
 *
 * Throws when the provider reports an `error` for this tab's attempt, or when
 * the code exchange fails. An `error` is held to the same `state` check as a
 * code (providers echo `state` on error redirects too), so a crafted link
 * can't put its own text in the login screen's error box.
 *
 * Whatever the outcome, the stored attempt is spent: calling again with the
 * same query returns null without touching the token endpoint.
 */
export async function completeRedirectLogin(
  search: string
): Promise<TokenResponse | null> {
  const params = new URLSearchParams(search)
  const code = params.get('code')
  const providerError = params.get('error')

  if (!code && !providerError) {
    return null
  }

  // Every path from here ends this attempt, so take it out of storage now,
  // before anything async: a second call — StrictMode, a remount, a reload that
  // kept the query — then finds nothing to match and cannot redeem the code.
  const pending = takePendingRedirectLogin()

  const state = params.get('state')
  if (
    !pending ||
    !state ||
    state !== pending.state ||
    Date.now() - pending.createdAt > REDIRECT_LOGIN_MAX_AGE_MS
  ) {
    return null
  }

  if (providerError) {
    throw new Error(params.get('error_description') || providerError)
  }

  if (!code) {
    return null
  }

  const config = getConfig()

  const body = new URLSearchParams({
    grant_type: 'authorization_code',
    code,
    redirect_uri: pending.redirectUri,
    client_id: config.clientId,
    code_verifier: pending.verifier,
  })

  const response = await fetch(config.tokenEndpoint, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body: body.toString(),
    signal: AbortSignal.timeout(10_000),
  })

  if (!response.ok) {
    const error: OIDCError = await response.json().catch(() => ({ error: '' }))
    throw new Error(error.error_description || error.error || 'Login failed')
  }

  return response.json()
}

/**
 * Read and remove the stored redirect attempt. Anything missing, unreadable or
 * malformed counts as "no attempt".
 */
function takePendingRedirectLogin(): PendingRedirectLogin | null {
  let raw: string | null
  try {
    raw = window.sessionStorage.getItem(REDIRECT_LOGIN_STORAGE_KEY)
    window.sessionStorage.removeItem(REDIRECT_LOGIN_STORAGE_KEY)
  } catch {
    return null
  }
  if (!raw) return null

  try {
    const parsed = JSON.parse(raw) as Partial<PendingRedirectLogin>
    if (
      typeof parsed.state === 'string' &&
      typeof parsed.verifier === 'string' &&
      typeof parsed.redirectUri === 'string' &&
      typeof parsed.createdAt === 'number'
    ) {
      return parsed as PendingRedirectLogin
    }
  } catch {
    // Unparseable — treated as absent below.
  }
  return null
}

function randomBase64Url(byteLength: number): string {
  return base64UrlEncode(crypto.getRandomValues(new Uint8Array(byteLength)))
}

async function sha256Base64Url(value: string): Promise<string> {
  // WebCrypto's digest only exists in secure contexts (HTTPS or localhost).
  if (!crypto.subtle) {
    throw new Error('Redirect login requires a secure context (HTTPS or localhost).')
  }
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(value))
  return base64UrlEncode(new Uint8Array(digest))
}

function base64UrlEncode(bytes: Uint8Array): string {
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
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
 * Get user info from OIDC provider
 */
export async function getUserInfo(): Promise<OIDCUserInfo> {
  const config = getConfig()
  const { token } = useAuthStore.getState()

  if (!token) {
    throw new Error('No access token available')
  }

  const response = await fetch(config.userinfoEndpoint, {
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
  const config = getConfig()
  const { refreshToken, clearAuth } = useAuthStore.getState()

  // Clear local auth state first (don't show "session expired" toast on manual logout)
  clearAuth(false)

  // If we have a refresh token and a logout URL, try to invalidate it
  if (refreshToken && config.endSessionEndpoint) {
    try {
      const params = new URLSearchParams({
        client_id: config.clientId,
        refresh_token: refreshToken,
      })

      await fetch(config.endSessionEndpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: params.toString(),
        signal: AbortSignal.timeout(10_000),
      })
    } catch (error) {
      // Ignore logout errors - we've already cleared local state
      console.warn('[Auth] Failed to logout from OIDC provider:', error)
    }
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
