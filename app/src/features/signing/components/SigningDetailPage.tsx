import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useParams, useNavigate } from '@tanstack/react-router'
import {
  ArrowLeft,
  RefreshCw,
  XCircle,
  Download,
  Calendar,
  Clock,
  Layers,
  ExternalLink,
  ChevronDown,
  ChevronRight,
  Loader2,
  Copy,
  Link,
  ShieldOff,
  CheckCircle2,
  User,
  Mail,
  FileText,
} from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { useToast } from '@/components/ui/use-toast'
import { useAppContextStore } from '@/stores/app-context-store'
import { usePermission } from '@/features/auth/hooks/usePermission'
import { SigningDocumentStatus } from '../types'
import { signingApi } from '../api/signing-api'
import {
  useSigningDocument,
  useRefreshDocument,
  useInvalidateTokens,
} from '../hooks/useSigningDocuments'
import { SigningStatusBadge } from './SigningStatusBadge'
import { RecipientTable } from './RecipientTable'
import { DocumentEventTimeline } from './DocumentEventTimeline'
import { CancelDocumentDialog } from './CancelDocumentDialog'
import { DeprecateDocumentDialog } from './DeprecateDocumentDialog'

const TERMINAL_STATUSES: string[] = [
  SigningDocumentStatus.COMPLETED,
  SigningDocumentStatus.DECLINED,
  SigningDocumentStatus.CANCELLED,
  SigningDocumentStatus.INVALIDATED,
  SigningDocumentStatus.ERROR,
]

function formatDate(dateString?: string): string {
  if (!dateString) return '-'
  return new Date(dateString).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function SigningDetailPage() {
  const { documentId, workspaceId } = useParams({
    from: '/workspace/$workspaceId/signing/$documentId',
  })
  const { t } = useTranslation()
  const { toast } = useToast()
  const { hasPermission, Permission } = usePermission()
  const navigate = useNavigate()
  const { currentWorkspace } = useAppContextStore()

  const [cancelDialogOpen, setCancelDialogOpen] = useState(false)
  const [deprecateDialogOpen, setDeprecateDialogOpen] = useState(false)
  const [eventsOpen, setEventsOpen] = useState(false)
  const [isDownloading, setIsDownloading] = useState(false)
  const [invalidateConfirmOpen, setInvalidateConfirmOpen] = useState(false)
  const [linkCopied, setLinkCopied] = useState(false)

  const { data: document, isLoading, error } = useSigningDocument(documentId)
  const refreshMutation = useRefreshDocument()
  const invalidateTokensMutation = useInvalidateTokens()

  const handleBackToList = () => {
    const wsId = currentWorkspace?.id ?? workspaceId
    navigate({
      to: '/workspace/$workspaceId/signing',
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- TanStack Router type limitation
      params: { workspaceId: wsId } as any,
    })
  }

  const handleRefresh = async () => {
    try {
      await refreshMutation.mutateAsync(documentId)
      toast({
        title: t('signing.detail.refreshed', 'Document status refreshed'),
      })
    } catch {
      toast({
        variant: 'destructive',
        title: t('common.error', 'Error'),
        description: t(
          'signing.detail.refreshError',
          'Failed to refresh document status',
        ),
      })
    }
  }

  const handleDownloadPDF = async () => {
    setIsDownloading(true)
    try {
      const blob = await signingApi.downloadPDF(documentId)
      const url = URL.createObjectURL(blob)
      const a = window.document.createElement('a')
      a.href = url
      a.download = `${document?.title ?? 'document'}.pdf`
      window.document.body.appendChild(a)
      a.click()
      window.document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch {
      toast({
        variant: 'destructive',
        title: t('common.error', 'Error'),
        description: t(
          'signing.detail.downloadError',
          'Failed to download PDF',
        ),
      })
    } finally {
      setIsDownloading(false)
    }
  }

  const publicDocUrl = `${window.location.origin}${import.meta.env.VITE_BASE_PATH || ''}/public/doc/${documentId}`

  const handleCopyLink = async () => {
    try {
      await navigator.clipboard.writeText(publicDocUrl)
      setLinkCopied(true)
      toast({
        title: t('signing.detail.linkCopied', 'Link copied to clipboard'),
      })
      setTimeout(() => setLinkCopied(false), 2000)
    } catch {
      toast({
        variant: 'destructive',
        title: t('common.error', 'Error'),
        description: t(
          'signing.detail.copyError',
          'Failed to copy link',
        ),
      })
    }
  }

  const handleInvalidateTokens = async () => {
    try {
      await invalidateTokensMutation.mutateAsync(documentId)
      setInvalidateConfirmOpen(false)
      toast({
        title: t(
          'signing.detail.tokensInvalidated',
          'Tokens invalidated successfully',
        ),
      })
    } catch {
      toast({
        variant: 'destructive',
        title: t('common.error', 'Error'),
        description: t(
          'signing.detail.invalidateError',
          'Failed to invalidate tokens',
        ),
      })
    }
  }

  const isTerminal = document
    ? TERMINAL_STATUSES.includes(document.status)
    : false
  const isCompleted = document?.status === SigningDocumentStatus.COMPLETED
  const canDeprecate =
    isCompleted && hasPermission(Permission.DOCUMENT_DEPRECATE)
  const isAwaitingInput =
    document?.status === SigningDocumentStatus.AWAITING_INPUT

  // Loading state
  if (isLoading) {
    return (
      <div className="flex h-full flex-1 flex-col bg-background">
        <header className="shrink-0 px-4 pb-6 pt-12 md:px-6 lg:px-6">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="mt-4 h-10 w-64" />
        </header>
        <div className="flex-1 px-4 pb-12 md:px-6 lg:px-6">
          <div className="grid gap-8 lg:grid-cols-[1fr_2fr]">
            <Skeleton className="h-48" />
            <Skeleton className="h-64" />
          </div>
        </div>
      </div>
    )
  }

  // Error state
  if (error || !document) {
    return (
      <div className="flex h-full flex-1 flex-col items-center justify-center bg-background">
        <p className="text-lg text-muted-foreground">
          {t('signing.detail.notFound', 'Document not found')}
        </p>
        <button
          onClick={handleBackToList}
          className="mt-4 text-sm text-foreground underline underline-offset-4 hover:no-underline"
        >
          {t('signing.detail.backToList', 'Back to Signing')}
        </button>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-1 flex-col bg-background">
      {/* Header */}
      <header className="shrink-0 px-4 pb-6 pt-12 md:px-6 lg:px-6">
        {/* Breadcrumb */}
        <button
          onClick={handleBackToList}
          className="mb-4 flex items-center gap-2 font-mono text-[10px] uppercase tracking-widest text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft size={14} />
          {t('signing.detail.backToList', 'Back to Signing')}
        </button>

        {/* Title + Status + Actions */}
        <div className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
          <div className="min-w-0 flex-1">
            <div className="mb-1 font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
              {t('signing.detail.header', 'Signing Document')}
            </div>
            <div className="flex items-center gap-3">
              <h1 className="font-display text-3xl font-light leading-tight tracking-tight text-foreground md:text-4xl">
                {document.title}
              </h1>
              <SigningStatusBadge status={document.status} />
            </div>
          </div>

          {/* Action buttons */}
          <div className="flex items-center gap-2">
            <button
              onClick={handleRefresh}
              disabled={refreshMutation.isPending}
              className="inline-flex items-center gap-2 rounded-none border border-border px-4 py-2.5 font-mono text-xs uppercase tracking-wider text-muted-foreground transition-colors hover:border-foreground hover:text-foreground disabled:opacity-50"
            >
              <RefreshCw
                size={14}
                className={refreshMutation.isPending ? 'animate-spin' : ''}
              />
              {t('signing.detail.refresh', 'Refresh')}
            </button>

            {isCompleted && (
              <button
                onClick={handleDownloadPDF}
                disabled={isDownloading}
                className="inline-flex items-center gap-2 rounded-none bg-foreground px-4 py-2.5 font-mono text-xs uppercase tracking-wider text-background transition-colors hover:bg-foreground/90 disabled:opacity-50"
              >
                {isDownloading ? (
                  <Loader2 size={14} className="animate-spin" />
                ) : (
                  <Download size={14} />
                )}
                {t('signing.detail.downloadPdf', 'Download PDF')}
              </button>
            )}

            {canDeprecate && (
              <button
                onClick={() => setDeprecateDialogOpen(true)}
                className="inline-flex items-center gap-2 rounded-none border border-destructive px-4 py-2.5 font-mono text-xs uppercase tracking-wider text-destructive transition-colors hover:bg-destructive hover:text-destructive-foreground"
              >
                <ShieldOff size={14} />
                {t('signing.detail.deprecate', 'Deprecate')}
              </button>
            )}

            {!isTerminal && (
              <button
                onClick={() => setCancelDialogOpen(true)}
                className="inline-flex items-center gap-2 rounded-none bg-destructive px-4 py-2.5 font-mono text-xs uppercase tracking-wider text-destructive-foreground transition-colors hover:bg-destructive/90"
              >
                <XCircle size={14} />
                {t('signing.detail.cancel', 'Cancel')}
              </button>
            )}
          </div>
        </div>
      </header>

      {/* Content */}
      <div className="flex-1 overflow-y-auto px-4 pb-12 md:px-6 lg:px-6">
        <div className="grid gap-8 lg:grid-cols-[1fr_2fr]">
          {/* Left Panel: Metadata */}
          <div className="space-y-6">
            <div className="border border-border bg-background p-6">
              <h2 className="mb-4 font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
                {t('signing.detail.metadata', 'Metadata')}
              </h2>

              <dl className="space-y-4">
                {/* Template Version */}
                <div>
                  <dt className="mb-1 flex items-center gap-1.5 text-xs text-muted-foreground">
                    <Layers size={12} />
                    {t('signing.detail.templateVersion', 'Template Version')}
                  </dt>
                  <dd className="font-mono text-sm text-foreground">
                    {document.templateVersionId}
                  </dd>
                </div>

                {/* External Reference */}
                {document.clientExternalReferenceId && (
                  <div>
                    <dt className="mb-1 flex items-center gap-1.5 text-xs text-muted-foreground">
                      <ExternalLink size={12} />
                      {t('signing.detail.externalRef', 'External Reference')}
                    </dt>
                    <dd className="font-mono text-sm text-foreground">
                      {document.clientExternalReferenceId}
                    </dd>
                  </div>
                )}

                {/* Signer Provider */}
                {document.signerProvider && (
                  <div>
                    <dt className="mb-1 flex items-center gap-1.5 text-xs text-muted-foreground">
                      <ExternalLink size={12} />
                      {t('signing.detail.provider', 'Provider')}
                    </dt>
                    <dd>
                      <span className="rounded-sm border px-1.5 py-0.5 font-mono text-[10px] uppercase text-muted-foreground">
                        {document.signerProvider}
                      </span>
                    </dd>
                  </div>
                )}

                {/* Created */}
                <div>
                  <dt className="mb-1 flex items-center gap-1.5 text-xs text-muted-foreground">
                    <Calendar size={12} />
                    {t('signing.detail.createdAt', 'Created')}
                  </dt>
                  <dd className="text-sm text-foreground">
                    {formatDate(document.createdAt)}
                  </dd>
                </div>

                {/* Last Updated */}
                <div>
                  <dt className="mb-1 flex items-center gap-1.5 text-xs text-muted-foreground">
                    <Clock size={12} />
                    {t('signing.detail.updatedAt', 'Last Updated')}
                  </dt>
                  <dd className="text-sm text-foreground">
                    {formatDate(document.updatedAt)}
                  </dd>
                </div>
              </dl>
            </div>

            {/* Public Document Link (AWAITING_INPUT) */}
            {isAwaitingInput && (
              <div className="border border-amber-500/30 bg-amber-500/5 p-6">
                <h2 className="mb-4 flex items-center gap-2 font-mono text-[10px] font-medium uppercase tracking-widest text-amber-600 dark:text-amber-400">
                  <Link size={12} />
                  {t('signing.detail.publicDocLink', 'Public Document Link')}
                </h2>

                {/* Recipient info */}
                {document.recipients.length > 0 && (
                  <div className="mb-4 space-y-2">
                    <div className="flex items-center gap-2 text-sm text-foreground">
                      <User size={14} className="text-muted-foreground" />
                      <span>{document.recipients[0].name}</span>
                    </div>
                    <div className="flex items-center gap-2 text-sm text-muted-foreground">
                      <Mail size={14} />
                      <span>{document.recipients[0].email}</span>
                    </div>
                  </div>
                )}

                {/* URL display */}
                <div className="mb-3 break-all rounded-sm border border-border bg-background p-3 font-mono text-xs text-muted-foreground">
                  {publicDocUrl}
                </div>

                {/* Actions */}
                <div className="flex flex-wrap gap-2">
                  <button
                    onClick={handleCopyLink}
                    className="inline-flex items-center gap-2 rounded-none border border-border px-3 py-2 font-mono text-[10px] uppercase tracking-wider text-muted-foreground transition-colors hover:border-foreground hover:text-foreground"
                  >
                    {linkCopied ? (
                      <CheckCircle2 size={12} />
                    ) : (
                      <Copy size={12} />
                    )}
                    {linkCopied
                      ? t('signing.detail.copied', 'Copied')
                      : t('signing.detail.copyLink', 'Copy Link')}
                  </button>

                  <button
                    onClick={() => setInvalidateConfirmOpen(true)}
                    disabled={invalidateTokensMutation.isPending}
                    className="inline-flex items-center gap-2 rounded-none border border-border px-3 py-2 font-mono text-[10px] uppercase tracking-wider text-muted-foreground transition-colors hover:border-foreground hover:text-foreground disabled:opacity-50"
                  >
                    <ShieldOff size={12} />
                    {t('signing.detail.invalidateTokens', 'Invalidate Tokens')}
                  </button>
                </div>

                {/* Invalidate confirmation */}
                {invalidateConfirmOpen && (
                  <div className="mt-3 rounded-sm border border-destructive/30 bg-destructive/5 p-3">
                    <p className="mb-3 text-sm text-destructive">
                      {t(
                        'signing.detail.invalidateWarning',
                        'This will invalidate all active access tokens. Recipients will need to request a new link via email.',
                      )}
                    </p>
                    <div className="flex gap-2">
                      <button
                        onClick={() => setInvalidateConfirmOpen(false)}
                        className="rounded-none border border-border px-3 py-1.5 font-mono text-[10px] uppercase tracking-wider text-muted-foreground transition-colors hover:border-foreground hover:text-foreground"
                      >
                        {t('common.cancel', 'Cancel')}
                      </button>
                      <button
                        onClick={handleInvalidateTokens}
                        disabled={invalidateTokensMutation.isPending}
                        className="inline-flex items-center gap-1 rounded-none bg-destructive px-3 py-1.5 font-mono text-[10px] uppercase tracking-wider text-destructive-foreground transition-colors hover:bg-destructive/90 disabled:opacity-50"
                      >
                        {invalidateTokensMutation.isPending && (
                          <Loader2 size={10} className="animate-spin" />
                        )}
                        {t('common.confirm', 'Confirm')}
                      </button>
                    </div>
                  </div>
                )}
              </div>
            )}

            {/* Field Responses (after signer submission) */}
            {document.fieldResponses && document.fieldResponses.length > 0 && (
              <div className="border border-border bg-background p-6">
                <h2 className="mb-4 flex items-center gap-2 font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
                  <FileText size={12} />
                  {t('signing.detail.fieldResponses', 'Field Responses')}
                </h2>
                <dl className="space-y-3">
                  {document.fieldResponses.map((response) => (
                    <div key={response.fieldId}>
                      <dt className="mb-0.5 text-xs text-muted-foreground">
                        {response.label}
                        <span className="ml-1.5 rounded-sm border px-1 py-0.5 font-mono text-[9px] uppercase text-muted-foreground/70">
                          {response.fieldType}
                        </span>
                      </dt>
                      <dd className="text-sm text-foreground">
                        {Array.isArray(response.value)
                          ? response.value.join(', ')
                          : String(response.value ?? '-')}
                      </dd>
                    </div>
                  ))}
                </dl>
              </div>
            )}
          </div>

          {/* Right Panel: Recipients + Events */}
          <div className="space-y-6">
            {/* Recipients */}
            <div className="border border-border bg-background">
              <div className="border-b border-border p-4">
                <h2 className="font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
                  {t('signing.detail.recipients', 'Recipients')}
                  <span className="ml-2 text-foreground">
                    ({document.recipients.length})
                  </span>
                </h2>
              </div>
              <RecipientTable
                documentId={documentId}
                recipients={document.recipients}
              />
            </div>

            {/* Events (collapsible) */}
            <div className="border border-border bg-background">
              <button
                onClick={() => setEventsOpen(!eventsOpen)}
                className="flex w-full items-center justify-between p-4 transition-colors hover:bg-accent"
              >
                <h2 className="font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
                  {t('signing.detail.events', 'Event Log')}
                </h2>
                {eventsOpen ? (
                  <ChevronDown size={16} className="text-muted-foreground" />
                ) : (
                  <ChevronRight size={16} className="text-muted-foreground" />
                )}
              </button>
              {eventsOpen && (
                <div className="border-t border-border px-4 pb-4">
                  <DocumentEventTimeline documentId={documentId} />
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Cancel Dialog */}
      <CancelDocumentDialog
        open={cancelDialogOpen}
        onOpenChange={setCancelDialogOpen}
        documentId={documentId}
        documentTitle={document.title}
        onSuccess={handleBackToList}
      />

      <DeprecateDocumentDialog
        open={deprecateDialogOpen}
        onOpenChange={setDeprecateDialogOpen}
        documentId={documentId}
        documentTitle={document.title}
        onSuccess={handleBackToList}
      />
    </div>
  )
}
