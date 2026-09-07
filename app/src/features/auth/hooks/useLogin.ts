import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { fetchMyRoles } from '@/features/auth/api/auth-api'
import { loginWithCredentials, getUserInfo } from '@/lib/oidc'
import { useAuthStore } from '@/stores/auth-store'

export interface UseLoginResult {
  username: string
  setUsername: (value: string) => void
  password: string
  setPassword: (value: string) => void
  isLoading: boolean
  error: string | null
  handleLogin: (e: React.FormEvent) => Promise<void>
}

/**
 * The credential login flow: authenticate, store tokens, load the profile and
 * roles, then continue to tenant selection.
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
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setIsLoading(true)

    try {
      const tokens = await loginWithCredentials(username, password)
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

  return {
    username,
    setUsername,
    password,
    setPassword,
    isLoading,
    error,
    handleLogin,
  }
}
