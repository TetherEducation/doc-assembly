import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Box,
  Loader2,
  AlertCircle,
  CheckCircle2,
  Mail,
  Send,
} from 'lucide-react'
import { LanguageSelector } from '@/components/common/LanguageSelector'
import { ThemeToggle } from '@/components/common/ThemeToggle'
import { changeLanguage } from '@/lib/i18n'
import {
  getDocumentAccessInfo,
  requestDocumentAccess,
  requestDocumentAccessFromToken,
} from '../api/public-signing-api'
import type { DocumentAccessInfo } from '../types'
import {
  publicLanguageOptions,
  type PublicSigningLanguage,
} from '../public-signing-language'

type AccessStatus = 'active' | 'completed' | 'expired'

interface AccessContext {
  title: string
  status: AccessStatus
}

type PageState =
  | { status: 'loading' }
  | { status: 'loaded'; context: AccessContext }
  | { status: 'submitting'; context: AccessContext }
  | { status: 'submitted'; context: AccessContext }
  | { status: 'error' }

interface PublicDocumentAccessPageProps {
  documentId?: string
  expiredToken?: string
  expiredMessage?: string
  language?: PublicSigningLanguage
}

export function PublicDocumentAccessPage({
  documentId,
  expiredToken,
  expiredMessage,
  language,
}: PublicDocumentAccessPageProps) {
  const { t } = useTranslation()
  const tokenMode = Boolean(expiredToken)
  const [state, setState] = useState<PageState>(() =>
    tokenMode
      ? {
          status: 'loaded',
          context: {
            title: t('publicSigning.access.documentTitle'),
            status: 'active',
          },
        }
      : { status: 'loading' },
  )
  const [email, setEmail] = useState('')
  const [cooldown, setCooldown] = useState(0)

  useEffect(() => {
    if (!language) return
    void changeLanguage(language)
  }, [language])

  useEffect(() => {
    if (tokenMode || !documentId) {
      return
    }

    (language
      ? getDocumentAccessInfo(documentId, publicLanguageOptions(language))
      : getDocumentAccessInfo(documentId))
      .then((info: DocumentAccessInfo) =>
        setState({
          status: 'loaded',
          context: {
            title: info.documentTitle,
            status: info.status,
          },
        }),
      )
      .catch(() => setState({ status: 'error' }))
  }, [tokenMode, documentId, language])

  useEffect(() => {
    if (cooldown <= 0) return
    const timer = setTimeout(() => setCooldown((c) => c - 1), 1000)
    return () => clearTimeout(timer)
  }, [cooldown])

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault()
      if (state.status !== 'loaded' && state.status !== 'submitted') return

      const context = state.context
      setState({ status: 'submitting', context })

      try {
        if (tokenMode && expiredToken) {
          if (language) {
            await requestDocumentAccessFromToken(
              expiredToken,
              email,
              publicLanguageOptions(language),
            )
          } else {
            await requestDocumentAccessFromToken(expiredToken, email)
          }
        } else if (documentId) {
          if (language) {
            await requestDocumentAccess(
              documentId,
              email,
              publicLanguageOptions(language),
            )
          } else {
            await requestDocumentAccess(documentId, email)
          }
        }
      } catch {
        // Always show success to prevent enumeration
      }

      setState({ status: 'submitted', context })
      setCooldown(60)
    },
    [state, tokenMode, expiredToken, documentId, email, language],
  )

  if (state.status === 'loading') {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="flex flex-col items-center gap-4">
          <Loader2 size={32} className="animate-spin text-primary" />
          <p className="text-sm text-muted-foreground">
            {t('publicSigning.loading')}
          </p>
        </div>
      </div>
    )
  }

  if (state.status === 'error') {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background px-6">
        <div className="mx-auto max-w-md text-center space-y-4">
          <AlertCircle size={48} className="mx-auto text-destructive" />
          <h1 className="text-xl font-semibold text-foreground">
            {t('publicSigning.access.notFound')}
          </h1>
          <p className="text-sm text-muted-foreground">
            {t('publicSigning.access.notFoundDescription')}
          </p>
        </div>
      </div>
    )
  }

  const context = state.context

  if (!tokenMode && context.status === 'expired') {
    return (
      <PageLayout title={context.title}>
        <div className="mx-auto max-w-md text-center space-y-4 py-12">
          <AlertCircle size={48} className="mx-auto text-muted-foreground" />
          <h1 className="text-xl font-semibold text-foreground">
            {t('publicSigning.access.unavailable')}
          </h1>
          <p className="text-sm text-muted-foreground">
            {t('publicSigning.access.unavailableDescription')}
          </p>
        </div>
      </PageLayout>
    )
  }

  return (
    <PageLayout title={context.title}>
      <div className="mx-auto max-w-md py-12 px-6">
        {state.status === 'submitted' ? (
          <div className="text-center space-y-4">
            <CheckCircle2 size={48} className="mx-auto text-green-600" />
            <h2 className="text-xl font-semibold text-foreground">
              {t('publicSigning.access.checkEmail')}
            </h2>
            <p className="text-sm text-muted-foreground">
              {t('publicSigning.access.checkEmailDescription')}
            </p>
            <button
              type="button"
              disabled={cooldown > 0}
              onClick={() => setState({ status: 'loaded', context })}
              className="mt-4 inline-flex items-center gap-2 rounded-md border border-border bg-background px-4 py-2 text-sm font-medium text-foreground transition-colors hover:bg-accent disabled:opacity-50"
            >
              {cooldown > 0
                ? t('publicSigning.access.requestAgainIn', {
                    seconds: cooldown,
                  })
                : t('publicSigning.access.requestAgain')}
            </button>
          </div>
        ) : (
          <div className="space-y-6">
            {tokenMode && (
              <div className="rounded-md border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-800 dark:text-amber-200">
                {expiredMessage || t('publicSigning.access.expiredNotice')}
              </div>
            )}

            <div className="text-center space-y-2">
              <Mail size={40} className="mx-auto text-primary" />
              <h2 className="text-xl font-semibold text-foreground">
                {tokenMode
                  ? t('publicSigning.access.expiredTitle')
                  : t('publicSigning.access.title')}
              </h2>
              <p className="text-sm text-muted-foreground">
                {tokenMode
                  ? t('publicSigning.access.expiredDescription')
                  : context.status === 'completed'
                    ? t('publicSigning.access.completedDescription')
                    : t('publicSigning.access.description')}
              </p>
            </div>

            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label
                  htmlFor="email"
                  className="block text-sm font-medium text-foreground mb-1.5"
                >
                  {t('publicSigning.access.emailLabel')}
                </label>
                <input
                  id="email"
                  type="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder={t('publicSigning.access.emailPlaceholder')}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                />
              </div>
              <button
                type="submit"
                disabled={state.status === 'submitting' || !email}
                className="inline-flex w-full items-center justify-center gap-2 rounded-md bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
              >
                {state.status === 'submitting' ? (
                  <>
                    <Loader2 size={16} className="animate-spin" />
                    {t('publicSigning.access.sending')}
                  </>
                ) : (
                  <>
                    <Send size={16} />
                    {t('publicSigning.access.sendLink')}
                  </>
                )}
              </button>
            </form>
          </div>
        )}
      </div>
    </PageLayout>
  )
}

function PageLayout({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-50 border-b border-border bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="mx-auto flex max-w-4xl items-center justify-between px-6 py-3">
          <div className="flex items-center gap-3">
            <div className="flex h-7 w-7 items-center justify-center border-2 border-foreground text-foreground">
              <Box size={14} fill="currentColor" />
            </div>
            <span className="font-display text-sm font-bold uppercase tracking-tight text-foreground">
              Doc-Assembly
            </span>
          </div>
          <div className="flex items-center gap-2">
            <LanguageSelector />
            <ThemeToggle />
          </div>
        </div>
      </header>
      <div className="mx-auto max-w-4xl px-6 py-4">
        <h1 className="text-lg font-semibold text-foreground">{title}</h1>
      </div>
      {children}
    </div>
  )
}
