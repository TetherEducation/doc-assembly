/**
 * Runtime auth configuration fetched from backend.
 */

// API Base URL derived from base path
const BASE_PATH = (import.meta.env.VITE_BASE_PATH || '').replace(/\/$/, '')
const API_BASE_URL = `${BASE_PATH}/api/v1`

export interface PanelProvider {
  name: string
  issuer: string
  /** Absent from backends that predate the redirect (authorization-code) login. */
  authorizationEndpoint?: string
  tokenEndpoint: string
  userinfoEndpoint: string
  endSessionEndpoint: string
  clientId: string
}

export interface AuthConfig {
  dummyAuth: boolean
  panelProvider?: PanelProvider
  features?: {
    gallery?: boolean
  }
}

let cachedConfig: AuthConfig | null = null
let fetchPromise: Promise<AuthConfig> | null = null

/**
 * Fetch auth configuration from backend.
 * Caches the result and deduplicates concurrent requests.
 * Falls back to dummy auth when VITE_DUMMY_AUTH=true and backend is unreachable.
 */
export async function getAuthConfig(): Promise<AuthConfig> {
  if (cachedConfig) {
    return cachedConfig
  }

  // Deduplicate concurrent requests
  if (fetchPromise) {
    return fetchPromise
  }

  fetchPromise = (async () => {
    try {
      const response = await fetch(`${API_BASE_URL}/config`, {
        signal: AbortSignal.timeout(10_000),
        headers: { 'Cache-Control': 'no-cache', Pragma: 'no-cache' },
      })
      if (!response.ok) {
        throw new Error(`Failed to fetch auth config: ${response.status}`)
      }
      cachedConfig = await response.json()
      return cachedConfig!
    } catch (error) {
      // Fallback: if VITE_DUMMY_AUTH is set, assume dummy auth even without backend
      if (import.meta.env.VITE_DUMMY_AUTH === 'true') {
        console.warn('[AuthConfig] Backend unreachable, using VITE_DUMMY_AUTH fallback')
        cachedConfig = { dummyAuth: true }
        return cachedConfig
      }
      throw error
    } finally {
      fetchPromise = null
    }
  })()

  return fetchPromise
}

/**
 * Get OIDC configuration for login/token operations.
 */
export function getOIDCConfig(config: AuthConfig): {
  authorizationEndpoint: string
  tokenEndpoint: string
  userinfoEndpoint: string
  endSessionEndpoint: string
  clientId: string
} {
  const panel = config.panelProvider

  return {
    authorizationEndpoint: panel?.authorizationEndpoint || '',
    tokenEndpoint: panel?.tokenEndpoint || '',
    userinfoEndpoint: panel?.userinfoEndpoint || '',
    endSessionEndpoint: panel?.endSessionEndpoint || '',
    clientId: panel?.clientId || '',
  }
}

/**
 * Clear cached config (useful for testing)
 */
export function clearAuthConfigCache(): void {
  cachedConfig = null
  fetchPromise = null
}
