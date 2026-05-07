import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertCircle, Eye, Loader2 } from 'lucide-react'
import { LanguageSelector } from '@/components/common/LanguageSelector'
import { ThemeToggle } from '@/components/common/ThemeToggle'
import { DocumentEditor } from '@/features/editor/components/DocumentEditor'
import { changeLanguage } from '@/lib/i18n'
import type { PublicSigningLanguage } from '@/features/public-signing/public-signing-language'
import {
  getReadOnlyView,
  getReadOnlyViewPdfUrl,
} from '../api/readonly-view-api'
import { ReadonlyPdfViewer } from './ReadonlyPdfViewer'
import type { ReadOnlyViewResponse } from '../types'

interface ReadonlyContractViewPageProps {
  token: string
  language?: PublicSigningLanguage
}

type PageState =
  | { status: 'loading' }
  | { status: 'loaded'; data: ReadOnlyViewResponse }
  | { status: 'error' }

export function ReadonlyContractViewPage({
  token,
  language,
}: ReadonlyContractViewPageProps) {
  const { t } = useTranslation()
  const [pageState, setPageState] = useState<PageState>({ status: 'loading' })

  useEffect(() => {
    if (!language) return
    void changeLanguage(language)
  }, [language])

  useEffect(() => {
    let cancelled = false

    async function load() {
      try {
        const data = await getReadOnlyView(token)
        if (!cancelled) {
          setPageState({ status: 'loaded', data })
        }
      } catch {
        if (!cancelled) {
          setPageState({ status: 'error' })
        }
      }
    }

    load()

    return () => {
      cancelled = true
    }
  }, [token])

  return (
    <div className="min-h-screen bg-background text-foreground">
      <header className="border-b border-border bg-background/95 px-4 py-3 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="mx-auto flex max-w-6xl items-center justify-between gap-3">
          <div>
            <div className="font-mono text-[10px] uppercase tracking-[0.22em] text-muted-foreground">
              {t('readonlyView.header', 'Read-only contract')}
            </div>
            <p className="text-xs text-muted-foreground">
              {t('readonlyView.noActions', 'Viewing only — no recipient actions are available.')}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <LanguageSelector />
            <ThemeToggle />
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-4 py-8">
        {pageState.status === 'loading' && <LoadingState />}
        {pageState.status === 'error' && <UnavailableState />}
        {pageState.status === 'loaded' && <LoadedState token={token} data={pageState.data} />}
      </main>
    </div>
  )
}

function LoadedState({ token, data }: { token: string; data: ReadOnlyViewResponse }) {
  if (data.mode === 'unavailable') {
    return <UnavailableState reason={data.reason} />
  }

  if (data.mode === 'pdf') {
    return (
      <ReadonlyShell data={data}>
        <ReadonlyPdfViewer
          pdfUrl={data.pdfUrl || getReadOnlyViewPdfUrl(token)}
          documentTitle={data.documentTitle}
        />
      </ReadonlyShell>
    )
  }

  return (
    <ReadonlyShell data={data}>
      <div className="overflow-hidden rounded-lg border border-border bg-background">
        <DocumentEditor
          initialContent={data.content ?? { type: 'doc', content: [] }}
          editable={false}
        />
      </div>
    </ReadonlyShell>
  )
}

function ReadonlyShell({
  data,
  children,
}: {
  data: ReadOnlyViewResponse
  children: React.ReactNode
}) {
  const { t } = useTranslation()
  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 md:flex-row md:items-start md:justify-between">
        <div className="min-w-0 space-y-2">
          <h1 className="font-display text-3xl font-light leading-tight tracking-tight md:text-4xl">
            {data.documentTitle}
          </h1>
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-sm border border-border px-2 py-1 font-mono text-[10px] uppercase tracking-wider text-muted-foreground">
              {data.documentStatus}
            </span>
            <span className="rounded-sm border border-emerald-500/30 bg-emerald-500/10 px-2 py-1 font-mono text-[10px] uppercase tracking-wider text-emerald-700 dark:text-emerald-300">
              {t('readonlyView.banner', 'Read-only view')}
            </span>
          </div>
        </div>
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          <Eye size={16} />
          <span>{t('readonlyView.viewOnly', 'View only')}</span>
        </div>
      </div>
      {children}
    </div>
  )
}

function LoadingState() {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-[360px] flex-col items-center justify-center gap-3 rounded-lg border border-border bg-card">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      <p className="font-mono text-xs uppercase tracking-wider text-muted-foreground">
        {t('readonlyView.loading', 'Loading contract')}
      </p>
    </div>
  )
}

function UnavailableState({ reason }: { reason?: string }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-[360px] flex-col items-center justify-center gap-4 rounded-lg border border-border bg-card p-8 text-center">
      <AlertCircle size={48} className="text-destructive" />
      <div className="space-y-1">
        <h1 className="text-xl font-semibold">
          {t('readonlyView.unavailableTitle', 'Contract unavailable')}
        </h1>
        <p className="text-sm text-muted-foreground">
          {reason || t('readonlyView.unavailableDescription', 'This read-only link is unavailable or has expired.')}
        </p>
      </div>
    </div>
  )
}
