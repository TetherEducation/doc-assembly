import { useState, useCallback, useMemo, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useEditor, EditorContent } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { TextStyle, FontFamily, FontSize } from '@tiptap/extension-text-style'
import { Color } from '@tiptap/extension-color'
import TextAlign from '@tiptap/extension-text-align'
import {
  Box,
  Loader2,
  AlertCircle,
  CheckCircle2,
  Clock,
  XCircle,
  Download,
} from 'lucide-react'
import { ReactNodeViewRenderer } from '@tiptap/react'
import axios from 'axios'
import { LanguageSelector } from '@/components/common/LanguageSelector'
import { ThemeToggle } from '@/components/common/ThemeToggle'
import { changeLanguage } from '@/lib/i18n'

import { InjectorExtension } from '@/features/editor/extensions/Injector'
import { SignatureExtension } from '@/features/editor/extensions/Signature'
import { ConditionalExtension } from '@/features/editor/extensions/Conditional'
import { ImageExtension } from '@/features/editor/extensions/Image'
import { PageBreakHR } from '@/features/editor/extensions/PageBreak'
import {
  TableExtension,
  TableRowExtension,
  TableHeaderExtension,
  TableCellExtension,
} from '@/features/editor/extensions/Table'
import { TableInjectorExtension } from '@/features/editor/extensions/TableInjector'
import { ListInjectorExtension } from '@/features/editor/extensions/ListInjector'
import { InteractiveFieldExtension } from '@/features/editor/extensions/InteractiveField'

import {
  getPublicSigningPage,
  submitPreSigningForm,
  proceedToSigning,
} from '../api/public-signing-api'
import { createPublicInteractiveFieldComponent } from './PublicInteractiveField'
import { PublicSignatureBlock } from './PublicSignatureBlock'
import { EmbeddedSigningFrame } from './EmbeddedSigningFrame'
import { PDFPreview } from './PDFPreview'
import { PublicDocumentAccessPage } from './PublicDocumentAccessPage'
import { FloatingSigningProgress, type SigningProgressTask } from './FloatingSigningProgress'
import type {
  PublicSigningResponse,
  FieldResponses,
  FieldResponsePayload,
} from '../types'
import {
  publicLanguageOptions,
  type PublicSigningLanguage,
} from '../public-signing-language'
import {
  EmbeddedModeContext,
  notifyParentSigningEvent,
  useEmbeddedMode,
  useEmbeddedThemeLock,
} from '../public-signing-embed'

type PageState =
  | { status: 'loading' }
  | { status: 'loaded'; data: PublicSigningResponse }
  | { status: 'submitting'; data: PublicSigningResponse }
  | { status: 'proceeding'; data: PublicSigningResponse }
  | { status: 'error'; code: string; message: string }

interface PublicSigningPageProps {
  token: string
  language?: PublicSigningLanguage
  /** Render chrome-less for host applications embedding this page. */
  embedded?: boolean
}

const uuidLikePattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i

function sanitizeDocumentTitle(title: string | undefined, fallback: string): string {
  const trimmed = (title ?? '').trim()
  if (!trimmed || uuidLikePattern.test(trimmed)) {
    return fallback
  }
  return trimmed
}

type PublicSigningField = NonNullable<PublicSigningResponse['form']>['fields'][number]

function isInteractiveFieldComplete(
  field: PublicSigningField,
  responses: FieldResponses,
): boolean {
  const resp = responses[field.id]
  if (!resp) return false

  if (field.fieldType === 'text') {
    const value = resp.response.text?.trim() ?? ''
    if (!value) return false
    return field.maxLength <= 0 || value.length <= field.maxLength
  }

  return (resp.response.selectedOptionIds?.length ?? 0) > 0
}

function buildSigningProgressTasks(
  form: NonNullable<PublicSigningResponse['form']>,
  responses: FieldResponses,
  agreed: boolean,
  agreementLabel: string,
  interactiveDescription: string,
): SigningProgressTask[] {
  return [
    ...form.fields.map((field) => ({
      id: field.id,
      label: field.label || interactiveDescription,
      description: interactiveDescription,
      completed: isInteractiveFieldComplete(field, responses),
      kind: 'interactive' as const,
    })),
    {
      id: 'agreement',
      label: agreementLabel,
      completed: agreed,
      kind: 'agreement' as const,
    },
  ]
}

export function PublicSigningPage({ token, language, embedded = false }: PublicSigningPageProps) {
  useEmbeddedThemeLock(embedded)

  return (
    <EmbeddedModeContext.Provider value={embedded}>
      <PublicSigningPageContent token={token} language={language} embedded={embedded} />
    </EmbeddedModeContext.Provider>
  )
}

function PublicSigningPageContent({ token, language, embedded }: PublicSigningPageProps) {
  const { t } = useTranslation()
  const [pageState, setPageState] = useState<PageState>({ status: 'loading' })
  const [responses, setResponses] = useState<FieldResponses>({})
  const [agreed, setAgreed] = useState(false)
  const [submitted, setSubmitted] = useState(false)
  const [validationErrors, setValidationErrors] = useState<Set<string>>(
    new Set(),
  )

  useEffect(() => {
    if (!language) return
    void changeLanguage(language)
  }, [language])

  // Load signing page state.
  useEffect(() => {
    let cancelled = false
    async function load() {
      try {
        const data = await getPublicSigningPage(
          token,
          publicLanguageOptions(language),
        )
        if (!cancelled) {
          setPageState({ status: 'loaded', data })
        }
      } catch (err) {
        if (cancelled) return
        handleLoadError(err, setPageState, t)
      }
    }
    load()
    return () => {
      cancelled = true
    }
  }, [token, t, language])

  const handleProcessingUpdate = useCallback((data: PublicSigningResponse) => {
    setPageState({ status: 'loaded', data })
  }, [])

  const handleProcessingError = useCallback(
    (err: unknown) => {
      handleLoadError(err, setPageState, t)
    },
    [t],
  )

  // Handle response changes.
  const handleResponseChange = useCallback(
    (
      fieldId: string,
      fieldType: string,
      response: { selectedOptionIds?: string[]; text?: string },
    ) => {
      setResponses((prev) => ({
        ...prev,
        [fieldId]: { fieldType, response },
      }))
      setValidationErrors((prev) => {
        if (!prev.has(fieldId)) return prev
        const next = new Set(prev)
        next.delete(fieldId)
        return next
      })
    },
    [],
  )

  // Validate fields.
  const validate = useCallback(
    (form: PublicSigningResponse): Set<string> => {
      const errors = new Set<string>()
      const fields = form.form?.fields ?? []
      for (const field of fields) {
        if (!field.required) continue
        const resp = responses[field.id]
        if (!resp) {
          errors.add(field.id)
          continue
        }
        if (field.fieldType === 'text') {
          if (!resp.response.text || resp.response.text.trim() === '') {
            errors.add(field.id)
          } else if (
            field.maxLength > 0 &&
            resp.response.text.length > field.maxLength
          ) {
            errors.add(field.id)
          }
        } else {
          if (
            !resp.response.selectedOptionIds ||
            resp.response.selectedOptionIds.length === 0
          ) {
            errors.add(field.id)
          }
        }
      }
      return errors
    },
    [responses],
  )

  // Handle form submit (Path B).
  const handleSubmit = useCallback(async () => {
    if (pageState.status !== 'loaded') return
    const data = pageState.data

    setSubmitted(true)
    const errors = validate(data)
    if (errors.size > 0) {
      setValidationErrors(errors)
      return
    }

    if (!agreed) return

    setPageState({ status: 'submitting', data })

    try {
      const fields = data.form?.fields ?? []
      const payload: FieldResponsePayload[] = fields.map((field) => {
        const resp = responses[field.id]
        return {
          fieldId: field.id,
          fieldType: field.fieldType,
          response: resp?.response ?? {},
        }
      })

      const result = await submitPreSigningForm(
        token,
        payload,
        publicLanguageOptions(language),
      )
      setPageState({ status: 'loaded', data: result })
    } catch (err) {
      handleSubmitError(err, setPageState, t)
    }
  }, [pageState, agreed, responses, token, validate, t, language])

  // Handle proceed to signing (Path A).
  const handleProceed = useCallback(async () => {
    if (pageState.status !== 'loaded') return

    setPageState({ status: 'proceeding', data: pageState.data })

    try {
      const result = await proceedToSigning(
        token,
        publicLanguageOptions(language),
      )
      setPageState({ status: 'loaded', data: result })
    } catch (err) {
      handleSubmitError(err, setPageState, t)
    }
  }, [pageState, token, t, language])

  // Handle signing completion.
  const handleSigningComplete = useCallback(async () => {
    try {
      const refreshed = await getPublicSigningPage(
        token,
        publicLanguageOptions(language),
      )
      setPageState({ status: 'loaded', data: refreshed })
      return
    } catch {
      // Fallback to local completed state if refresh fails.
    }

    setPageState({
      status: 'loaded',
      data: {
        step: 'completed',
        documentTitle:
          pageState.status === 'loaded' ? pageState.data.documentTitle : '',
        recipientName:
          pageState.status === 'loaded' ? pageState.data.recipientName : '',
        hasCurrentUserSigned: true,
        canSign: false,
        canDownload: false,
      },
    })
  }, [pageState, token, language])

  // Handle signing decline.
  const handleSigningDecline = useCallback(async () => {
    try {
      const refreshed = await getPublicSigningPage(
        token,
        publicLanguageOptions(language),
      )
      setPageState({ status: 'loaded', data: refreshed })
      return
    } catch {
      // Fallback to local declined state if refresh fails.
    }

    setPageState({
      status: 'loaded',
      data: {
        step: 'declined',
        documentTitle:
          pageState.status === 'loaded' ? pageState.data.documentTitle : '',
        recipientName:
          pageState.status === 'loaded' ? pageState.data.recipientName : '',
        hasCurrentUserSigned: pageState.status === 'loaded' ? pageState.data.hasCurrentUserSigned : false,
        canSign: false,
        canDownload: false,
      },
    })
  }, [pageState, token, language])

  // Embedded hosts need to know when the signing reaches a terminal state
  // (to refresh requirement status and dismiss their modal). Notify once.
  const parentNotifiedRef = useRef(false)
  useEffect(() => {
    if (!embedded || parentNotifiedRef.current) return
    if (pageState.status !== 'loaded') return

    const { step, hasCurrentUserSigned } = pageState.data
    // 'waiting' + hasCurrentUserSigned means THIS signer is done and only
    // other recipients remain — per-recipient completion for the host,
    // matching the semantics hosts expect from provider embeds.
    if (step === 'completed' || (step === 'waiting' && hasCurrentUserSigned)) {
      parentNotifiedRef.current = true
      notifyParentSigningEvent('embed.form.completed')
    } else if (step === 'declined') {
      parentNotifiedRef.current = true
      notifyParentSigningEvent('embed.form.exception', { reason: 'declined' })
    }
  }, [embedded, pageState])

  // --- RENDER ---

  if (pageState.status === 'loading') {
    return <LoadingScreen />
  }

  if (pageState.status === 'error') {
    if (pageState.code === 'EXPIRED') {
      return (
        <PublicDocumentAccessPage
          expiredToken={token}
          expiredMessage={pageState.message}
          language={language}
        />
      )
    }
    return <ErrorScreen code={pageState.code} message={pageState.message} />
  }

  const data =
    pageState.status === 'loaded'
      ? pageState.data
      : pageState.status === 'submitting' || pageState.status === 'proceeding'
        ? pageState.data
        : null

  if (!data) return <LoadingScreen />
  const displayDocumentTitle = sanitizeDocumentTitle(
    data.documentTitle,
    t('publicSigning.access.documentTitle'),
  )

  if (data.step === 'processing') {
    return (
      <ProcessingScreen
        documentTitle={displayDocumentTitle}
        response={data}
        token={token}
        language={language}
        onUpdate={handleProcessingUpdate}
        onError={handleProcessingError}
      />
    )
  }

  if (data.step === 'document_updated') {
    return <DocumentUpdatedScreen documentTitle={displayDocumentTitle} />
  }

  if (data.step === 'unavailable') {
    return <UnavailableScreen documentTitle={displayDocumentTitle} />
  }

  // Step: completed.
  if (data.step === 'completed') {
    return (
      <CompletedScreen
        documentTitle={displayDocumentTitle}
        canDownload={data.canDownload}
        downloadUrl={data.downloadUrl}
      />
    )
  }

  // Step: declined.
  if (data.step === 'declined') {
    return <DeclinedScreen documentTitle={displayDocumentTitle} />
  }

  // Step: waiting for previous signers.
  if (data.step === 'waiting') {
    return (
      <WaitingScreen
        documentTitle={displayDocumentTitle}
        recipientName={data.recipientName}
        position={data.signingPosition ?? 0}
        total={data.totalSigners ?? 0}
        waitingForPrevious={data.waitingForPrevious ?? !data.hasCurrentUserSigned}
        token={token}
        language={language}
        onReady={(newData) => setPageState({ status: 'loaded', data: newData })}
      />
    )
  }

  // Step: signing (embedded iframe).
  if (data.step === 'signing') {
    if (data.fallbackUrl) {
      return <FallbackRedirect url={data.fallbackUrl} />
    }

    return (
      <PageShell documentTitle={displayDocumentTitle}>
        <EmbeddedSigningFrame
          url={data.embeddedSigningUrl!}
          token={token}
          language={language}
          onComplete={handleSigningComplete}
          onDecline={handleSigningDecline}
        />
      </PageShell>
    )
  }

  // Step: preview.
  // Path B: has form with interactive fields.
  if (data.form && data.form.fields.length > 0) {
    const isSubmitting = pageState.status === 'submitting'
    const allowedFieldIds = new Set(data.form.fields.map((field) => field.id))
    const showValidationSummary = submitted && validationErrors.size > 0
    const progressTasks = buildSigningProgressTasks(
      data.form,
      responses,
      agreed,
      t('publicSigning.guidance.agreementTask'),
      t('publicSigning.guidance.interactiveTaskDescription'),
    )
    const handleTaskSelect = (task: SigningProgressTask) => {
      const field = document.getElementById(`public-field-${task.id}`)
      field?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }

    return (
      <PageShell documentTitle={displayDocumentTitle}>
        <div className="mx-auto max-w-4xl px-6 pt-8 pb-32">
          <div className="mb-8 space-y-2">
            <h1 className="text-2xl font-semibold text-foreground">
              {displayDocumentTitle}
            </h1>
            <div className="text-sm text-muted-foreground">
              <span>
                {t('publicSigning.to')}: {data.recipientName}
              </span>
            </div>
          </div>

          {/* Document content (TipTap read-only editor) */}
          <div className="mb-8 rounded-lg border border-border bg-card shadow-sm">
            <div className="p-8">
              <DocumentViewer
                content={data.form.content}
                signerRoleId={data.form.roleId}
                allowedFieldIds={allowedFieldIds}
                responses={responses}
                onResponseChange={handleResponseChange}
                validationErrors={validationErrors}
                submitted={submitted}
              />
            </div>
          </div>

          {showValidationSummary && (
            <div className="mb-8 flex items-center gap-2 rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
              <AlertCircle size={16} />
              <span>
                {t('publicSigning.validationSummary', {
                  count: validationErrors.size,
                })}
              </span>
            </div>
          )}
        </div>

        <FloatingSigningProgress
          tasks={progressTasks}
          actionLabel={t('publicSigning.guidance.action')}
          expandedActionLabel={t('publicSigning.guidance.expandedAction')}
          onAction={handleSubmit}
          onTaskSelect={handleTaskSelect}
          onAgreementChange={setAgreed}
          disabledReason={t('publicSigning.guidance.disabledReason')}
          loading={isSubmitting}
        />
      </PageShell>
    )
  }

  // Path A / Path B after submit: show real PDF preview with proceed button.
  return (
    <PageShell documentTitle={displayDocumentTitle}>
      <div className="mx-auto max-w-4xl px-6 py-8">
        <PDFPreview
          token={token}
          language={language}
          documentTitle={displayDocumentTitle}
          recipientName={data.recipientName}
          onProceed={handleProceed}
          isLoading={pageState.status === 'proceeding'}
        />
      </div>

      {/* Loading overlay while uploading to provider */}
      {pageState.status === 'proceeding' && <ProceedingOverlay />}
    </PageShell>
  )
}

// --- Shell & Sub-components ---

function PageShell({
  children,
}: {
  documentTitle?: string
  children: React.ReactNode
}) {
  const embedded = useEmbeddedMode()

  // Embedded: the host application owns all chrome (title, close button,
  // language). Render only the content on the page background.
  if (embedded) {
    return <div className="min-h-screen bg-background">{children}</div>
  }

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

      {children}

      <footer className="border-t border-border py-2 text-center">
        <span className="font-mono text-[9px] uppercase tracking-widest text-muted-foreground/40">
          Doc-Assembly
        </span>
      </footer>
    </div>
  )
}

// Known limitation: navigating to the provider's standalone page unloads this
// document and with it the embedded-host bridge — the host receives no
// completion event for sessions that fall back. Hosts relying on the bridge
// should also poll their own backend state.
function FallbackRedirect({ url }: { url: string }) {
  useEffect(() => {
    window.location.href = url
  }, [url])
  return <LoadingScreen />
}

function LoadingScreen() {
  const { t } = useTranslation()
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

function ErrorScreen({ code, message }: { code: string; message: string }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6">
      <div className="mx-auto max-w-md text-center space-y-4">
        <AlertCircle size={48} className="mx-auto text-destructive" />
        <h1 className="text-xl font-semibold text-foreground">
          {code === 'EXPIRED'
            ? t('publicSigning.errors.expiredTitle')
            : code === 'ALREADY_USED'
              ? t('publicSigning.errors.alreadyUsedTitle')
              : code === 'NOT_FOUND'
                ? t('publicSigning.errors.invalidTokenTitle')
                : t('publicSigning.errors.errorTitle')}
        </h1>
        <p className="text-sm text-muted-foreground">{message}</p>
      </div>
    </div>
  )
}

function ProcessingScreen({
  documentTitle,
  response,
  token,
  language,
  onUpdate,
  onError,
}: {
  documentTitle: string
  response: PublicSigningResponse
  token: string
  language?: PublicSigningLanguage
  onUpdate: (data: PublicSigningResponse) => void
  onError: (err: unknown) => void
}) {
  const { t } = useTranslation()
  const isRecovering = response.processingReason === 'recovering_provider_submission'
  const retryAfterMs = Math.max(1, response.retryAfterSeconds ?? 3) * 1_000

  useEffect(() => {
    let cancelled = false
    let inFlight = false

    const poll = async () => {
      if (cancelled || inFlight) return
      inFlight = true
      try {
        const data = await getPublicSigningPage(
          token,
          publicLanguageOptions(language),
        )
        if (!cancelled) {
          onUpdate(data)
        }
      } catch (err) {
        if (!cancelled) {
          onError(err)
        }
      } finally {
        inFlight = false
      }
    }

    const interval = window.setInterval(poll, retryAfterMs)

    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [onError, onUpdate, retryAfterMs, token, language])

  return (
    <PageShell documentTitle={documentTitle}>
      <div className="mx-auto flex min-h-[70vh] max-w-md flex-col items-center justify-center px-6 text-center">
        <Loader2 size={36} className="mb-4 animate-spin text-primary" />
        <h1 className="text-xl font-semibold text-foreground">
          {t(isRecovering ? 'publicSigning.processing.recoveringTitle' : 'publicSigning.processing.title')}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">
          {t(isRecovering ? 'publicSigning.processing.recoveringMessage' : 'publicSigning.processing.message')}
        </p>
        {isRecovering && response.supportCode ? (
          <p className="mt-3 font-mono text-xs text-muted-foreground">
            {t('publicSigning.processing.supportCode', { code: response.supportCode })}
          </p>
        ) : null}
      </div>
    </PageShell>
  )
}

function DocumentUpdatedScreen({ documentTitle }: { documentTitle: string }) {
  const { t } = useTranslation()
  return (
    <PageShell documentTitle={documentTitle}>
      <div className="mx-auto flex min-h-[70vh] max-w-md flex-col items-center justify-center px-6 text-center">
        <AlertCircle size={40} className="mb-4 text-amber-600" />
        <h1 className="text-xl font-semibold text-foreground">
          {t('publicSigning.documentUpdated.title')}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">
          {t('publicSigning.documentUpdated.message')}
        </p>
      </div>
    </PageShell>
  )
}

function UnavailableScreen({ documentTitle }: { documentTitle: string }) {
  const { t } = useTranslation()
  return (
    <PageShell documentTitle={documentTitle}>
      <div className="mx-auto flex min-h-[70vh] max-w-md flex-col items-center justify-center px-6 text-center">
        <XCircle size={40} className="mb-4 text-destructive" />
        <h1 className="text-xl font-semibold text-foreground">
          {t('publicSigning.unavailable.title')}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">
          {t('publicSigning.unavailable.message')}
        </p>
      </div>
    </PageShell>
  )
}

function CompletedScreen({
  documentTitle,
  canDownload,
  downloadUrl,
}: {
  documentTitle: string
  canDownload: boolean
  downloadUrl?: string
}) {
  const { t } = useTranslation()
  return (
    <PageShell documentTitle={documentTitle}>
      <div className="mx-auto max-w-4xl px-6 py-8 space-y-6">
        <div className="rounded-lg border border-border bg-card p-6">
          <div className="flex items-start gap-4">
            <CheckCircle2 size={36} className="text-green-600 mt-0.5" />
            <div className="space-y-2">
              <h1 className="text-xl font-semibold text-foreground">
                {t('publicSigning.completed.title')}
              </h1>
              <p className="text-sm text-muted-foreground">
                {t('publicSigning.completed.message', { title: documentTitle })}
              </p>
            </div>
          </div>

          {canDownload && downloadUrl && (
            <div className="mt-6 flex justify-center">
              <a
                href={downloadUrl}
                className="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
              >
                <Download size={16} />
                {t('publicSigning.completed.download')}
              </a>
            </div>
          )}
        </div>

      </div>
    </PageShell>
  )
}

function DeclinedScreen({ documentTitle }: { documentTitle: string }) {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6">
      <div className="mx-auto max-w-md text-center space-y-4">
        <XCircle size={48} className="mx-auto text-destructive" />
        <h1 className="text-xl font-semibold text-foreground">
          {t('publicSigning.declined.title')}
        </h1>
        <p className="text-sm text-muted-foreground">
          {t('publicSigning.declined.message', { title: documentTitle })}
        </p>
      </div>
    </div>
  )
}

function WaitingScreen({
  documentTitle,
  recipientName,
  position,
  total,
  waitingForPrevious,
  token,
  language,
  onReady,
}: {
  documentTitle: string
  recipientName: string
  position: number
  total: number
  waitingForPrevious: boolean
  token: string
  language?: PublicSigningLanguage
  onReady: (data: PublicSigningResponse) => void
}) {
  const { t } = useTranslation()

  // Poll every 30s to check if previous signers completed.
  useEffect(() => {
    const interval = setInterval(async () => {
      try {
        const res = await getPublicSigningPage(
          token,
          publicLanguageOptions(language),
        )
        if (res.step !== 'waiting') {
          onReady(res)
        }
      } catch {
        // Ignore — will retry next interval.
      }
    }, 30_000)

    return () => clearInterval(interval)
  }, [token, onReady, language])

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6">
      <div className="mx-auto max-w-md text-center space-y-4">
        <Clock size={48} className="mx-auto text-muted-foreground" />
        <h1 className="text-xl font-semibold text-foreground">
          {t(
            waitingForPrevious
              ? 'publicSigning.waiting.title'
              : 'publicSigning.waiting.afterSigningTitle',
          )}
        </h1>
        <p className="text-sm text-muted-foreground">
          {t(
            waitingForPrevious
              ? 'publicSigning.waiting.message'
              : 'publicSigning.waiting.afterSigningMessage',
            {
              name: recipientName,
              position,
              total,
              title: documentTitle,
            },
          )}
        </p>
        <div className="flex items-center justify-center gap-2 text-xs text-muted-foreground/60">
          <Loader2 size={14} className="animate-spin" />
          <span>{t('publicSigning.waiting.autoRefresh')}</span>
        </div>
      </div>
    </div>
  )
}

function ProceedingOverlay() {
  const { t } = useTranslation()
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/80 backdrop-blur-sm">
      <div className="flex flex-col items-center gap-4 text-center">
        <Loader2 size={40} className="animate-spin text-primary" />
        <p className="text-base font-medium text-foreground">
          {t('publicSigning.proceeding.title')}
        </p>
        <p className="text-sm text-muted-foreground">
          {t('publicSigning.proceeding.warning')}
        </p>
      </div>
    </div>
  )
}

// --- Document Viewer (TipTap read-only) ---

interface DocumentViewerProps {
  content: Record<string, unknown>
  signerRoleId: string
  allowedFieldIds: Set<string>
  responses: FieldResponses
  onResponseChange: (
    fieldId: string,
    fieldType: string,
    response: { selectedOptionIds?: string[]; text?: string },
  ) => void
  validationErrors: Set<string>
  submitted: boolean
}

function DocumentViewer({
  content,
  signerRoleId,
  allowedFieldIds,
  responses,
  onResponseChange,
  validationErrors,
  submitted,
}: DocumentViewerProps) {
  const editorContainerRef = useRef<HTMLDivElement>(null)
  const runtimeStateRef = useRef({
    responses,
    validationErrors,
    submitted,
  })
  const allowedFieldIdsRef = useRef(allowedFieldIds)

  useEffect(() => {
    runtimeStateRef.current = {
      responses,
      validationErrors,
      submitted,
    }
  }, [responses, validationErrors, submitted])

  useEffect(() => {
    allowedFieldIdsRef.current = allowedFieldIds
  }, [allowedFieldIds])

  const PublicFieldComponent = useMemo(
    () =>
      // eslint-disable-next-line react-hooks/refs
      createPublicInteractiveFieldComponent({
        signerRoleId,
        allowedFieldIdsRef,
        runtimeStateRef,
        onResponseChange,
      }),
    [signerRoleId, onResponseChange],
  )

  const PublicInteractiveFieldExtension = useMemo(
    () =>
      InteractiveFieldExtension.extend({
        addNodeView() {
          return ReactNodeViewRenderer(PublicFieldComponent, {
            stopEvent: () => true,
          })
        },
        addKeyboardShortcuts() {
          return {}
        },
        addCommands() {
          return {}
        },
      }),
    [PublicFieldComponent],
  )

  const PublicSignatureExtension = useMemo(
    () =>
      SignatureExtension.extend({
        addNodeView() {
          return ReactNodeViewRenderer(PublicSignatureBlock, {
            stopEvent: () => true,
          })
        },
        addKeyboardShortcuts() {
          return {}
        },
        addCommands() {
          return {}
        },
      }),
    [],
  )

  const editor = useEditor(
    {
      immediatelyRender: false,
      editable: false,
      content: content as Record<string, unknown>,
      extensions: [
        StarterKit.configure({
          heading: { levels: [1, 2, 3] },
        }),
        TextStyle,
        Color,
        FontFamily.configure({ types: ['textStyle'] }),
        FontSize.configure({ types: ['textStyle'] }),
        TextAlign.configure({
          types: ['heading', 'paragraph', 'tableCell', 'tableHeader'],
        }),
        InjectorExtension,
        PublicSignatureExtension,
        ConditionalExtension,
        ImageExtension,
        PageBreakHR,
        TableExtension.configure({
          resizable: false,
          lastColumnResizable: false,
        }),
        TableRowExtension,
        TableHeaderExtension,
        TableCellExtension,
        TableInjectorExtension,
        ListInjectorExtension,
        PublicInteractiveFieldExtension,
      ],
      editorProps: {
        attributes: {
          class:
            'prose prose-sm dark:prose-invert max-w-none focus:outline-none',
        },
      },
    },
    [PublicInteractiveFieldExtension],
  )

  useEffect(() => {
    if (!editor) return
    editor.view.updateState(editor.view.state)
  }, [editor, responses, validationErrors, submitted])

  if (!editor) {
    return (
      <div className="flex items-center justify-center h-32">
        <Loader2 size={24} className="animate-spin text-primary" />
      </div>
    )
  }

  return (
    <div ref={editorContainerRef}>
      <EditorContent editor={editor} />
    </div>
  )
}

// --- Error handlers ---

function handleLoadError(
  err: unknown,
  setPageState: React.Dispatch<React.SetStateAction<PageState>>,
  t: (key: string) => string,
) {
  if (axios.isAxiosError(err)) {
    const status = err.response?.status
    const body = err.response?.data as
      | { code?: string; message?: string; error?: string }
      | undefined
    const errMsg = body?.message || body?.error || ''
    if (errMsg.includes('expired')) {
      setPageState({
        status: 'error',
        code: 'EXPIRED',
        message: errMsg || t('publicSigning.errors.expired'),
      })
    } else if (errMsg.includes('already been used')) {
      setPageState({
        status: 'error',
        code: 'ALREADY_USED',
        message: errMsg || t('publicSigning.errors.alreadyUsed'),
      })
    } else if (status === 404) {
      setPageState({
        status: 'error',
        code: 'NOT_FOUND',
        message: errMsg || t('publicSigning.errors.invalidToken'),
      })
    } else if (status === 401) {
      setPageState({
        status: 'error',
        code: 'NOT_FOUND',
        message: errMsg || t('publicSigning.errors.invalidToken'),
      })
    } else {
      setPageState({
        status: 'error',
        code: 'SERVER_ERROR',
        message: errMsg || t('publicSigning.errors.serverError'),
      })
    }
  } else {
    setPageState({
      status: 'error',
      code: 'UNKNOWN',
      message: t('publicSigning.errors.serverError'),
    })
  }
}

function handleSubmitError(
  err: unknown,
  setPageState: React.Dispatch<React.SetStateAction<PageState>>,
  t: (key: string) => string,
) {
  if (axios.isAxiosError(err)) {
    const body = err.response?.data as { message?: string } | undefined
    setPageState({
      status: 'error',
      code: 'SUBMIT_ERROR',
      message: body?.message || t('publicSigning.errors.submitFailed'),
    })
  } else {
    setPageState({
      status: 'error',
      code: 'UNKNOWN',
      message: t('publicSigning.errors.submitFailed'),
    })
  }
}
