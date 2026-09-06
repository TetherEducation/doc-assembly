import { APP_NAME } from '@/lib/brand'
import { useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { ArrowRight, Box, Loader2, AlertCircle } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import { loginWithCredentials, getUserInfo } from '@/lib/oidc'
import { fetchMyRoles } from '@/features/auth/api/auth-api'
import { LanguageSelector } from '@/components/common/LanguageSelector'
import { ThemeToggle } from '@/components/common/ThemeToggle'

export const Route = createFileRoute('/login')({
  component: LoginPage,
})

function LoginPage() {
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
      // Authenticate with Keycloak
      const tokens = await loginWithCredentials(username, password)

      // Store tokens
      setTokens(tokens.access_token, tokens.refresh_token, tokens.expires_in)

      // Get user info from Keycloak
      const userInfo = await getUserInfo()
      setUserProfile({
        id: userInfo.sub,
        email: userInfo.email || '',
        firstName: userInfo.given_name,
        lastName: userInfo.family_name,
        username: userInfo.preferred_username,
      })

      // Fetch roles from backend API
      try {
        const roles = await fetchMyRoles()
        setAllRoles(roles)
      } catch (rolesError) {
        console.warn('[Auth] Failed to fetch roles:', rolesError)
        // Continue without roles - user can still access basic features
      }

      // Navigate to tenant selection
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

  return (
    <div className="relative flex min-h-screen flex-col justify-center overflow-hidden bg-background">
      <div className="absolute top-6 right-6 flex items-center gap-2">
        <LanguageSelector />
        <ThemeToggle />
      </div>
      <div className="mx-auto flex h-full w-full max-w-7xl flex-col justify-center px-6 md:px-12 lg:px-32">
        <div className="mb-16 max-w-2xl md:mb-20">
          <div className="mb-10 flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center border-2 border-foreground text-foreground">
              <Box size={16} fill="currentColor" />
            </div>
            <span className="font-display text-lg font-bold uppercase tracking-tight text-foreground">
              {APP_NAME}
            </span>
          </div>

          <h1 className="font-display text-5xl font-light leading-[1.05] tracking-tight text-foreground md:text-6xl lg:text-7xl">
            {t('login.title', 'Login to')}
            <br />
            <span className="font-semibold">{t('login.subtitle', 'your account.')}</span>
          </h1>
        </div>

        <div className="w-full max-w-[400px]">
          <form className="space-y-12" onSubmit={handleLogin}>
            {error && (
              <div className="flex items-center gap-3 rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
                <AlertCircle size={18} />
                <span>{error}</span>
              </div>
            )}

            <div className="space-y-8">
              <div className="group">
                <label className="mb-2 block font-mono text-xs font-medium uppercase tracking-widest text-muted-foreground transition-colors group-focus-within:text-foreground">
                  {t('login.email', 'Username / Email')}
                </label>
                <input
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  required
                  disabled={isLoading}
                  autoComplete="username"
                  className="w-full rounded-none border-0 border-b-2 border-border bg-transparent py-3 font-light text-xl text-foreground outline-none transition-all placeholder:text-muted focus-visible:border-foreground focus-visible:ring-0 disabled:opacity-50"
                />
              </div>
              <div className="group">
                <label className="mb-2 block font-mono text-xs font-medium uppercase tracking-widest text-muted-foreground transition-colors group-focus-within:text-foreground">
                  {t('login.password', 'Password')}
                </label>
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  required
                  disabled={isLoading}
                  autoComplete="current-password"
                  className="w-full rounded-none border-0 border-b-2 border-border bg-transparent py-3 font-light text-xl text-foreground outline-none transition-all placeholder:text-muted focus-visible:border-foreground focus-visible:ring-0 disabled:opacity-50"
                />
              </div>
            </div>

            <div className="flex flex-col items-start gap-8 pt-4">
              <button
                type="submit"
                disabled={isLoading || !username || !password}
                className="group flex h-14 w-full items-center justify-between gap-3 rounded-none bg-foreground px-8 text-sm font-medium tracking-wide text-background transition-colors hover:bg-foreground/90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {isLoading ? (
                  <>
                    <span>{t('login.authenticating', 'AUTHENTICATING...')}</span>
                    <Loader2 size={18} className="animate-spin" />
                  </>
                ) : (
                  <>
                    <span>{t('login.authenticate', 'AUTHENTICATE')}</span>
                    <ArrowRight size={18} className="transition-transform group-hover:translate-x-1" />
                  </>
                )}
              </button>
              <a
                href="#"
                className="border-b border-transparent pb-0.5 font-mono text-xs text-muted-foreground transition-colors hover:border-foreground hover:text-foreground"
              >
                {t('login.recover', 'Recover password access')}
              </a>
            </div>
          </form>
        </div>

        <div className="absolute bottom-12 left-6 font-mono text-[10px] uppercase tracking-widest text-muted-foreground/50 md:left-12 lg:left-32">
          v2.4 — Secure Environment
        </div>
      </div>
    </div>
  )
}
