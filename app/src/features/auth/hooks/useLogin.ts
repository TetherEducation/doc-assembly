import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { completeLogin } from '@/features/auth/complete-login'
import { loginWithCredentials } from '@/lib/oidc'

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
 * The credential login flow: authenticate, then hand the tokens to completeLogin
 * (access check, profile, roles, tokens last) and continue to tenant selection.
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
      const outcome = await completeLogin(tokens)

      if (outcome.kind === 'not-invited') {
        setError(
          t(
            'login.notInvited',
            "This account hasn't been added yet. Ask an administrator to invite you."
          )
        )
        return
      }
      if (outcome.kind === 'rejected') {
        setError(t('login.rejected', "Your sign-in couldn't be verified. Please try again."))
        return
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
