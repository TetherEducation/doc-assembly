import { apiClient } from '@/lib/api-client'
import { endSession, getUserInfo, parseJwtPayload, type TokenResponse } from '@/lib/oidc'
import { useAuthStore, type RoleEntry, type UserProfile } from '@/stores/auth-store'

/** The parts of the auth store a login writes. */
export interface LoginStore {
  setUserProfile: (profile: UserProfile | null) => void
  setAllRoles: (roles: RoleEntry[]) => void
  setTokens: (accessToken: string, refreshToken: string, expiresIn: number) => void
}

export type LoginOutcome =
  | { kind: 'signed-in' }
  /** A valid identity with no account in this app: nobody has added them. */
  | { kind: 'not-invited'; email: string }
  /** The API does not accept the token at all. */
  | { kind: 'rejected'; status: number }

export interface CompleteLoginOptions {
  apiBase?: string
  store?: LoginStore
  fetchImpl?: typeof fetch
}

/**
 * Everything a login does once it holds tokens, whichever way they were obtained:
 * the password grant in useLogin, or a wrapper's own flow (an OIDC redirect, for
 * example). Wrappers should call this rather than copy it.
 *
 * Order matters. Access is checked and the profile read with the new token
 * BEFORE anything is stored, and the tokens are stored last: a stored token
 * makes the root route guard leave /login at once, so anything after it would
 * run on a page that is already gone and its outcome would never be shown.
 *
 * A login that is turned away stores nothing and ends its OIDC session, so the
 * next attempt asks again instead of silently reusing the same identity.
 */
export async function completeLogin(
  tokens: Pick<TokenResponse, 'access_token' | 'refresh_token' | 'expires_in'>,
  options: CompleteLoginOptions = {}
): Promise<LoginOutcome> {
  const fetchImpl = options.fetchImpl ?? fetch
  const store = options.store ?? useAuthStore.getState()
  const apiBase = (options.apiBase ?? apiClient.defaults.baseURL ?? '/api/v1').replace(/\/$/, '')

  const access = await checkAccess(apiBase, tokens.access_token, fetchImpl)
  if (access.kind !== 'ok') {
    await endSession(tokens.refresh_token, fetchImpl)
    if (access.kind === 'not-invited') {
      const email = parseJwtPayload(tokens.access_token)?.email
      return { kind: 'not-invited', email: typeof email === 'string' ? email : '' }
    }
    return access
  }

  const userInfo = await getUserInfo(tokens.access_token, fetchImpl)

  store.setUserProfile({
    id: userInfo.sub,
    email: userInfo.email || '',
    firstName: userInfo.given_name,
    lastName: userInfo.family_name,
    username: userInfo.preferred_username,
  })
  store.setAllRoles(access.roles)
  store.setTokens(tokens.access_token, tokens.refresh_token, tokens.expires_in)
  return { kind: 'signed-in' }
}

type AccessCheck =
  | { kind: 'ok'; roles: RoleEntry[] }
  | { kind: 'not-invited' }
  | { kind: 'rejected'; status: number }

/**
 * Asks the API what the new token may do. 403 {"error": "user not found"} is the
 * identity middleware's answer to a valid login with no account; 401 means the
 * token is not accepted. Any other failure lets the user in with no roles: roles
 * are not required to get in, a user without them just sees fewer features.
 *
 * Uses fetch rather than apiClient on purpose: apiClient attaches the stored
 * token and answers a 401 by trying to refresh it, and nothing is stored yet.
 */
async function checkAccess(
  apiBase: string,
  accessToken: string,
  fetchImpl: typeof fetch
): Promise<AccessCheck> {
  try {
    const response = await fetchImpl(`${apiBase}/me/roles`, {
      headers: { Authorization: `Bearer ${accessToken}` },
      signal: AbortSignal.timeout(10_000),
    })
    const body = (await response.json().catch(() => ({}))) as {
      roles?: RoleEntry[]
      error?: unknown
    }
    if (response.status === 403 && body.error === 'user not found') return { kind: 'not-invited' }
    if (response.status === 401) return { kind: 'rejected', status: 401 }
    if (response.ok && Array.isArray(body.roles)) return { kind: 'ok', roles: body.roles }
    console.warn('[Auth] Failed to fetch roles:', response.status)
  } catch (err) {
    console.warn('[Auth] Failed to fetch roles:', err)
  }
  return { kind: 'ok', roles: [] }
}
