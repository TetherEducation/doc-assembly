import { useEffect, useEffectEvent, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { fetchMyRoles } from '@/features/auth/api/auth-api'
import {
  completeRedirectLogin,
  getUserInfo,
  loginWithCredentials,
  startRedirectLogin as beginRedirectLogin,
  type TokenResponse,
} from '@/lib/oidc'
import { useAuthStore } from '@/stores/auth-store'

export interface UseLoginResult {
  username: string
  setUsername: (value: string) => void
  password: string
  setPassword: (value: string) => void
  /**
   * True while a password login is in flight, and while the return leg of a
   * redirect login is being completed (from the first render after the
   * provider sends the browser back).
   */
  isLoading: boolean
  error: string | null
  handleLogin: (e: React.FormEvent) => Promise<void>
  /**
   * Send the browser to the identity provider (authorization code + PKCE).
   * `idpHint` skips the provider's own login page and goes straight to that
   * federated provider, e.g. `Google`. The return leg is handled by this hook
   * when the login route mounts again — the caller does nothing more.
   */
  startRedirectLogin: (options?: { idpHint?: string }) => Promise<void>
  /** True once a redirect login has been started, until the page navigates away (or it fails). */
  isRedirecting: boolean
}

/**
 * Query parameters the provider puts on the login URL when it sends the browser
 * back. All of them are removed once read, so a reload can't replay them.
 */
const CALLBACK_PARAMS = ['code', 'state', 'session_state', 'iss', 'error', 'error_description']

function isRedirectCallback(search: string): boolean {
  const params = new URLSearchParams(search)
  return params.has('code') || params.has('error')
}

function stripCallbackParams(): void {
  const url = new URL(window.location.href)
  const present = CALLBACK_PARAMS.filter((name) => url.searchParams.has(name))
  if (present.length === 0) return

  present.forEach((name) => url.searchParams.delete(name))
  // Keep the router's history state; only the URL changes.
  window.history.replaceState(window.history.state, '', url.toString())
}

/**
 * The login flows: authenticate, store tokens, load the profile and roles, then
 * continue to tenant selection. Two ways in — username/password
 * (`handleLogin`), or a redirect to the identity provider
 * (`startRedirectLogin`, completed automatically when the provider sends the
 * browser back to the login route) — sharing every step after the tokens
 * arrive, so the two can't drift.
 *
 * Extracted from the login route so a wrapper can present its own login screen
 * without copying any of this. That matters because this path handles tokens
 * and role loading — duplicating it would mean a fix here silently not reaching
 * whichever copy the wrapper ships. Wrappers replace the route's markup; the
 * behaviour stays here, in one place.
 */
export function useLogin(): UseLoginResult {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { setTokens, setUserProfile, setAllRoles } = useAuthStore()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  // Start loading on the very first render when the provider has just sent the
  // browser back, so the screen doesn't flash an idle form during the exchange.
  const [isLoading, setIsLoading] = useState(() => isRedirectCallback(window.location.search))
  const [isRedirecting, setIsRedirecting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const callbackHandled = useRef(false)

  const completeLogin = async (tokens: TokenResponse) => {
    setTokens(tokens.access_token, tokens.refresh_token, tokens.expires_in)

    const userInfo = await getUserInfo()
    setUserProfile({
      id: userInfo.sub,
      email: userInfo.email || '',
      firstName: userInfo.given_name,
      lastName: userInfo.family_name,
      username: userInfo.preferred_username,
    })

    // Roles are not required to get in — a user without them still reaches the
    // app, just with fewer features — so a failure here must not block login.
    try {
      const roles = await fetchMyRoles()
      setAllRoles(roles)
    } catch (rolesError) {
      console.warn('[Auth] Failed to fetch roles:', rolesError)
    }

    navigate({ to: '/select-tenant' })
  }

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setIsLoading(true)

    try {
      const tokens = await loginWithCredentials(username, password)
      await completeLogin(tokens)
    } catch (err) {
      console.error('[Auth] Login failed:', err)
      setError(
        err instanceof Error
          ? err.message
          : t('login.error', 'Invalid username or password')
      )
    } finally {
      setIsLoading(false)
    }
  }

  const startRedirectLogin = async (options?: { idpHint?: string }) => {
    setError(null)
    setIsRedirecting(true)

    try {
      await beginRedirectLogin({ idpHint: options?.idpHint })
      // Stay "redirecting": the browser is on its way to the provider.
    } catch (err) {
      console.error('[Auth] Redirect login failed to start:', err)
      setError(
        err instanceof Error
          ? err.message
          : t('login.redirectError', 'Sign-in failed. Please try again.')
      )
      setIsRedirecting(false)
    }
  }

  const handleRedirectCallback = useEffectEvent(async () => {
    const search = window.location.search

    try {
      if (!isRedirectCallback(search)) return

      // The query is captured, and the code is single-use whatever happens
      // next, so drop it from the URL before anything async: a reload can't
      // replay it.
      stripCallbackParams()

      const tokens = await completeRedirectLogin(search)
      if (tokens) {
        await completeLogin(tokens)
      }
    } catch (err) {
      console.error('[Auth] Redirect login failed:', err)
      setError(
        err instanceof Error
          ? err.message
          : t('login.redirectError', 'Sign-in failed. Please try again.')
      )
    } finally {
      setIsLoading(false)
    }
  })

  useEffect(() => {
    // Once per mount, even under StrictMode's double-invoked effects, so a
    // code is never sent to the token endpoint twice.
    if (callbackHandled.current) return
    callbackHandled.current = true
    handleRedirectCallback()
  }, [])

  return {
    username,
    setUsername,
    password,
    setPassword,
    isLoading,
    error,
    handleLogin,
    startRedirectLogin,
    isRedirecting,
  }
}
