import { useEffect, useState, useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, AlertCircle, CheckCircle2 } from 'lucide-react'
import {
  getPublicSigningPage,
  completeEmbeddedSigning,
} from '../api/public-signing-api'
import {
  publicLanguageOptions,
  type PublicSigningLanguage,
} from '../public-signing-language'

interface EmbeddedSigningFrameProps {
  url: string
  token: string
  language?: PublicSigningLanguage
  onComplete: () => void
  onDecline: () => void
}

const FINALIZING_MIN_VISIBLE_MS = 3_000
const FINALIZING_SUCCESS_VISIBLE_MS = 700

/**
 * EmbeddedSigningFrame renders the signing provider inside an iframe.
 * It is 100% provider-agnostic: it never imports or references any provider.
 *
 * Two mechanisms detect signing completion:
 * 1. postMessage from our own callback bridge page (loaded when the provider redirects)
 * 2. Polling fallback every 10s (works with any provider, especially those without redirect)
 */
export function EmbeddedSigningFrame({
  url,
  token,
  language,
  onComplete,
  onDecline,
}: EmbeddedSigningFrameProps) {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const [finalizing, setFinalizing] = useState(false)
  const [finalizingSucceeded, setFinalizingSucceeded] = useState(false)
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const completionStartedRef = useRef(false)
  const finalizingStartedAtRef = useRef<number | null>(null)
  const finalizingFinishedRef = useRef(false)
  const finalizingTimeoutsRef = useRef<number[]>([])

  const clearFinalizingTimeouts = useCallback(() => {
    for (const timeout of finalizingTimeoutsRef.current) {
      window.clearTimeout(timeout)
    }
    finalizingTimeoutsRef.current = []
  }, [])

  useEffect(() => {
    return clearFinalizingTimeouts
  }, [clearFinalizingTimeouts])

  const startFinalizing = useCallback(() => {
    clearFinalizingTimeouts()
    finalizingStartedAtRef.current = Date.now()
    finalizingFinishedRef.current = false
    setFinalizingSucceeded(false)
    setFinalizing(true)
  }, [clearFinalizingTimeouts])

  const finishFinalizing = useCallback((finish: () => void) => {
    if (finalizingFinishedRef.current) return
    finalizingFinishedRef.current = true

    const elapsed =
      finalizingStartedAtRef.current == null
        ? FINALIZING_MIN_VISIBLE_MS
        : Date.now() - finalizingStartedAtRef.current
    const remaining = Math.max(0, FINALIZING_MIN_VISIBLE_MS - elapsed)

    const successTimeout = window.setTimeout(() => {
      setFinalizingSucceeded(true)

      const finishTimeout = window.setTimeout(() => {
        finish()
      }, FINALIZING_SUCCESS_VISIBLE_MS)
      finalizingTimeoutsRef.current.push(finishTimeout)
    }, remaining)
    finalizingTimeoutsRef.current.push(successTimeout)
  }, [])

  const handleComplete = useCallback(async () => {
    if (completionStartedRef.current) return
    completionStartedRef.current = true
    startFinalizing()
    try {
      if (language) {
        await completeEmbeddedSigning(token, publicLanguageOptions(language))
      } else {
        await completeEmbeddedSigning(token)
      }
    } catch {
      // Best effort — the backend may already have marked it.
    }
  }, [startFinalizing, token, language])

  // 1. Listen for postMessage — callback bridge (own origin) + provider native events.
  useEffect(() => {
    const handler = (e: MessageEvent) => {
      // Mechanism 1: Callback bridge (our own origin)
      if (e.origin === window.location.origin && e.data?.type === 'SIGNING_EVENT') {
        if (e.data.status === 'signed') handleComplete()
        if (e.data.status === 'declined') onDecline()
        return
      }

      // Mechanism 2: Native postMessage from the signing provider iframe.
      // Provider-agnostic: only trust messages whose source is our iframe,
      // then look for generic completion/rejection strings in the event type.
      if (e.source === iframeRef.current?.contentWindow) {
        const eventType = e.data?.type || e.data?.event || e.data?.action || ''
        if (typeof eventType === 'string') {
          const normalizedEventType = eventType.toLowerCase()
          if (
            normalizedEventType === 'document-completed' ||
            normalizedEventType === 'document.completed' ||
            normalizedEventType === 'document_completed' ||
            normalizedEventType === 'completed'
          ) {
            handleComplete()
          }
          if (
            normalizedEventType === 'document-rejected' ||
            normalizedEventType === 'document.rejected' ||
            normalizedEventType === 'document_rejected' ||
            normalizedEventType === 'rejected' ||
            normalizedEventType === 'declined'
          ) {
            onDecline()
          }
        }
      }
    }

    window.addEventListener('message', handler)
    return () => window.removeEventListener('message', handler)
  }, [handleComplete, onDecline])

  // 2. Polling fallback — universal, works with ANY provider.
  useEffect(() => {
    const interval = setInterval(async () => {
      try {
        const res = language
          ? await getPublicSigningPage(token, publicLanguageOptions(language))
          : await getPublicSigningPage(token)
        if (res.step === 'completed') {
          handleComplete()
        } else if (res.step === 'declined') {
          onDecline()
        }
      } catch {
        // Token may have been used — ignore and let next poll handle it.
      }
    }, 10_000)

    return () => clearInterval(interval)
  }, [token, handleComplete, onDecline, language])

  // 3. Once the provider accepts the final signature, keep the user in a clear
  // finalization state and poll faster until doc-assembly exposes the terminal
  // completed/download state.
  useEffect(() => {
    if (!finalizing) return

    let cancelled = false
    let inFlight = false

    const pollFinalState = async () => {
      if (cancelled || inFlight) return
      inFlight = true
      try {
        const res = language
          ? await getPublicSigningPage(token, publicLanguageOptions(language))
          : await getPublicSigningPage(token)
        if (cancelled) return
        if (res.step === 'completed' || (res.step === 'waiting' && res.hasCurrentUserSigned)) {
          finishFinalizing(onComplete)
        } else if (res.step === 'declined') {
          onDecline()
        }
      } catch {
        // Keep the finalization message visible and retry.
      } finally {
        inFlight = false
      }
    }

    pollFinalState()
    const interval = setInterval(pollFinalState, 2_000)

    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [finalizing, finishFinalizing, onComplete, onDecline, token, language])

  return (
    <div className="relative w-full" style={{ minHeight: 'calc(100vh - 105px)' }}>
      {loading && (
        <div className="absolute inset-0 flex items-center justify-center bg-background/50 z-10">
          <div className="flex flex-col items-center gap-3">
            <Loader2 size={32} className="animate-spin text-primary" />
            <p className="text-sm text-muted-foreground">
              {t('publicSigning.loadingSigning')}
            </p>
          </div>
        </div>
      )}

      {finalizing && (
        <div
          data-state="open"
          className="absolute inset-0 z-20 flex items-center justify-center bg-background/95 px-6 text-center backdrop-blur-sm data-[state=open]:animate-in data-[state=open]:fade-in-0"
        >
          <div className="mx-auto flex max-w-sm flex-col items-center gap-4 rounded-xl border border-border bg-card p-7 shadow-lg duration-200 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95">
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10 text-primary transition-colors duration-300">
              {finalizingSucceeded ? (
                <CheckCircle2 size={30} className="text-green-600 animate-in zoom-in-75 duration-200" />
              ) : (
                <Loader2 size={28} className="animate-spin" />
              )}
            </div>
            <h2 className="text-lg font-semibold text-foreground">
              {t(
                finalizingSucceeded
                  ? 'publicSigning.finalizing.successTitle'
                  : 'publicSigning.finalizing.title',
              )}
            </h2>
            <p className="text-sm text-muted-foreground">
              {t(
                finalizingSucceeded
                  ? 'publicSigning.finalizing.successMessage'
                  : 'publicSigning.finalizing.message',
              )}
            </p>
          </div>
        </div>
      )}

      {error && !finalizing && (
        <div className="absolute inset-0 flex items-center justify-center bg-background z-10">
          <div className="flex flex-col items-center gap-3 text-center px-6">
            <AlertCircle size={32} className="text-destructive" />
            <p className="text-sm text-muted-foreground">
              {t('publicSigning.errors.iframeLoadFailed')}
            </p>
          </div>
        </div>
      )}

      <iframe
        ref={iframeRef}
        src={url}
        title="Document Signing"
        className="w-full border-0"
        style={{ height: 'calc(100vh - 105px)' }}
        sandbox="allow-scripts allow-forms allow-same-origin allow-top-navigation"
        onLoad={() => setLoading(false)}
        onError={() => {
          setLoading(false)
          setError(true)
        }}
      />
    </div>
  )
}
