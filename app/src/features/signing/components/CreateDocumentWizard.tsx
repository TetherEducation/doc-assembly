import { useState, useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import {
  X,
  ChevronLeft,
  ChevronRight,
  Send,
  Copy,
  CheckCircle2,
  Clock,
} from 'lucide-react'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import { cn } from '@/lib/utils'
import { SigningDocumentStatus } from '../types'
import type { SigningDocumentDetail } from '../types'
import { useTemplateWithVersions } from '@/features/templates/hooks/useTemplateDetail'
import { useCreateDocument } from '../hooks/useSigningDocuments'
import type {
  CreateDocumentRequest,
  DocumentRecipientCommand,
} from '../types'
import { WizardStepVersion } from './WizardStepVersion'
import { WizardStepValues } from './WizardStepValues'
import { WizardStepRecipients } from './WizardStepRecipients'
import { WizardStepReview } from './WizardStepReview'

const STEPS = ['version', 'values', 'recipients', 'review'] as const
type Step = (typeof STEPS)[number]

interface CreateDocumentWizardProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess?: (documentId: string) => void
}

export function CreateDocumentWizard({
  open,
  onOpenChange,
  onSuccess,
}: CreateDocumentWizardProps) {
  const { t } = useTranslation()
  const createDocument = useCreateDocument()

  // Wizard state
  const [step, setStep] = useState<Step>('version')
  const [templateId, setTemplateId] = useState('')
  const [versionId, setVersionId] = useState('')
  const [title, setTitle] = useState('')
  const [values, setValues] = useState<Record<string, unknown>>({})
  const [recipients, setRecipients] = useState<DocumentRecipientCommand[]>([])
  const [awaitingInputResult, setAwaitingInputResult] =
    useState<SigningDocumentDetail | null>(null)
  const [resultLinkCopied, setResultLinkCopied] = useState(false)

  const { data: templateDetail } = useTemplateWithVersions(templateId)

  const selectedVersion = useMemo(
    () => templateDetail?.versions?.find((v) => v.id === versionId) ?? null,
    [templateDetail, versionId]
  )

  const injectables = useMemo(
    () => selectedVersion?.injectables ?? [],
    [selectedVersion]
  )
  const signerRoles = useMemo(
    () => selectedVersion?.signerRoles ?? [],
    [selectedVersion]
  )

  // Values typed for another template or version stay in state (so going
  // back restores them), but only the selected version's keys are reviewed
  // and sent.
  const injectedValues = useMemo(() => {
    const keys = new Set(injectables.map((i) => i.definition.key))
    return Object.fromEntries(
      Object.entries(values).filter(([key]) => keys.has(key))
    )
  }, [injectables, values])

  const signerRoleNames = useMemo(() => {
    const map: Record<string, string> = {}
    for (const role of signerRoles) {
      map[role.id] = role.roleName
    }
    return map
  }, [signerRoles])

  const stepIndex = STEPS.indexOf(step)

  // Reset form when the dialog opens. The parent opens it through the `open`
  // prop, which never calls onOpenChange(true), so adjust state on the prop.
  const [prevOpen, setPrevOpen] = useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    if (open) {
      setStep('version')
      setTemplateId('')
      setVersionId('')
      setTitle('')
      setValues({})
      setRecipients([])
      setAwaitingInputResult(null)
      setResultLinkCopied(false)
    }
  }

  // Initialize recipients when version changes and we move to recipients step
  const initRecipients = useCallback(() => {
    if (
      signerRoles.length > 0 &&
      (recipients.length === 0 ||
        recipients[0]?.roleId !== signerRoles[0]?.id)
    ) {
      setRecipients(
        signerRoles
          .sort((a, b) => a.signerOrder - b.signerOrder)
          .map((role) => ({
            roleId: role.id,
            name: '',
            email: '',
          }))
      )
    }
  }, [signerRoles, recipients])

  const canProceed = useMemo(() => {
    switch (step) {
      case 'version':
        return !!templateId && !!versionId && !!title.trim()
      case 'values': {
        const requiredInjectables = injectables.filter((i) => i.isRequired)
        return requiredInjectables.every((i) => {
          const val = values[i.definition.key]
          return val !== undefined && val !== '' && val !== null
        })
      }
      case 'recipients':
        return (
          signerRoles.length === 0 ||
          recipients.every((r) => r.name.trim() && r.email.trim())
        )
      case 'review':
        return true
      default:
        return false
    }
  }, [step, templateId, versionId, title, injectables, values, signerRoles, recipients])

  const handleNext = () => {
    if (step === 'values') {
      initRecipients()
    }
    if (stepIndex < STEPS.length - 1) {
      setStep(STEPS[stepIndex + 1])
    }
  }

  const handleBack = () => {
    if (stepIndex > 0) {
      setStep(STEPS[stepIndex - 1])
    }
  }

  const handleCopyResultLink = async () => {
    if (!awaitingInputResult?.preSigningUrl) return
    try {
      await navigator.clipboard.writeText(awaitingInputResult.preSigningUrl)
      setResultLinkCopied(true)
      setTimeout(() => setResultLinkCopied(false), 2000)
    } catch {
      // Silently fail
    }
  }

  const handleSubmit = async () => {
    const request: CreateDocumentRequest = {
      templateVersionId: versionId,
      title: title.trim(),
      injectedValues,
      recipients,
    }

    try {
      const doc = await createDocument.mutateAsync(request)
      if (
        doc.status === SigningDocumentStatus.AWAITING_INPUT &&
        doc.preSigningUrl
      ) {
        setAwaitingInputResult(doc)
      } else {
        onOpenChange(false)
        onSuccess?.(doc.id)
      }
    } catch {
      // Error handled by mutation
    }
  }

  const stepLabels: Record<Step, string> = {
    version: t('signing.wizard.stepVersion', 'Template & Version'),
    values: t('signing.wizard.stepValues', 'Values'),
    recipients: t('signing.wizard.stepRecipients', 'Recipients'),
    review: t('signing.wizard.stepReview', 'Review'),
  }

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-black/80 data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0" />
        <DialogPrimitive.Content
          className={cn(
            'fixed left-[50%] top-[50%] z-50 w-full max-w-2xl translate-x-[-50%] translate-y-[-50%] border border-border bg-background p-0 shadow-lg duration-200',
            'data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95'
          )}
        >
          {/* Header */}
          <div className="flex items-start justify-between border-b border-border p-6">
            <div>
              <DialogPrimitive.Title className="font-mono text-sm font-medium uppercase tracking-widest text-foreground">
                {t('signing.wizard.title', 'Create Signing Document')}
              </DialogPrimitive.Title>
              <DialogPrimitive.Description className="mt-1 text-sm font-light text-muted-foreground">
                {stepLabels[step]}
              </DialogPrimitive.Description>
            </div>
            <DialogPrimitive.Close className="text-muted-foreground transition-colors hover:text-foreground">
              <X className="h-5 w-5" />
              <span className="sr-only">Close</span>
            </DialogPrimitive.Close>
          </div>

          {/* Step indicator */}
          <div className="flex border-b border-border">
            {STEPS.map((s, i) => (
              <div
                key={s}
                className={cn(
                  'flex-1 py-2 text-center font-mono text-[10px] uppercase tracking-widest transition-colors',
                  i <= stepIndex
                    ? 'bg-foreground/5 text-foreground'
                    : 'text-muted-foreground/50'
                )}
              >
                <span className="mr-1">{i + 1}.</span>
                {stepLabels[s]}
              </div>
            ))}
          </div>

          {/* Body */}
          <div className="max-h-[60vh] overflow-y-auto p-6">
            {/* Awaiting Input Success Screen */}
            {awaitingInputResult && (
              <div className="space-y-6">
                <div className="flex items-start gap-3">
                  <div className="mt-0.5 rounded-full bg-amber-500/10 p-2">
                    <Clock size={20} className="text-amber-600 dark:text-amber-400" />
                  </div>
                  <div>
                    <h3 className="font-mono text-sm font-medium uppercase tracking-wider text-foreground">
                      {t(
                        'signing.wizard.awaitingInputTitle',
                        'Waiting for Signer Input',
                      )}
                    </h3>
                    <p className="mt-1 text-sm font-light text-muted-foreground">
                      {t(
                        'signing.wizard.awaitingInputMessage',
                        'Document created. Waiting for signer to complete interactive fields.',
                      )}
                    </p>
                  </div>
                </div>

                {/* Pre-signing URL */}
                <div className="space-y-3">
                  <label className="font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
                    {t('signing.wizard.preSigningUrl', 'Pre-Signing URL')}
                  </label>
                  <div className="break-all rounded-sm border border-border bg-muted/50 p-3 font-mono text-xs text-muted-foreground">
                    {awaitingInputResult.preSigningUrl}
                  </div>
                  <button
                    type="button"
                    onClick={handleCopyResultLink}
                    className="inline-flex items-center gap-2 rounded-none border border-border px-4 py-2.5 font-mono text-xs uppercase tracking-wider text-muted-foreground transition-colors hover:border-foreground hover:text-foreground"
                  >
                    {resultLinkCopied ? (
                      <CheckCircle2 size={14} />
                    ) : (
                      <Copy size={14} />
                    )}
                    {resultLinkCopied
                      ? t('signing.detail.copied', 'Copied')
                      : t('signing.detail.copyLink', 'Copy Link')}
                  </button>
                </div>
              </div>
            )}

            {/* Title field (shown in version step) */}
            {!awaitingInputResult && step === 'version' && (
              <div className="mb-6">
                <label
                  htmlFor="document-title"
                  className="mb-2 block font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground"
                >
                  {t('signing.wizard.documentTitle', 'Document Title')}
                  <span className="ml-1 text-destructive">*</span>
                </label>
                <input
                  id="document-title"
                  type="text"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  placeholder={t(
                    'signing.wizard.documentTitlePlaceholder',
                    'Enter document title...'
                  )}
                  maxLength={255}
                  autoFocus
                  className="w-full rounded-none border-0 border-b border-border bg-transparent py-2 text-base font-light text-foreground outline-none transition-all placeholder:text-muted-foreground/50 focus-visible:border-foreground focus-visible:ring-0"
                />
              </div>
            )}

            {!awaitingInputResult && step === 'version' && (
              <WizardStepVersion
                templateId={templateId}
                versionId={versionId}
                onTemplateChange={setTemplateId}
                onVersionChange={setVersionId}
              />
            )}
            {!awaitingInputResult && step === 'values' && (
              <WizardStepValues
                injectables={injectables}
                values={values}
                onValuesChange={setValues}
              />
            )}
            {!awaitingInputResult && step === 'recipients' && (
              <WizardStepRecipients
                signerRoles={signerRoles}
                recipients={recipients}
                onRecipientsChange={setRecipients}
              />
            )}
            {!awaitingInputResult && step === 'review' && (
              <WizardStepReview
                templateTitle={templateDetail?.title ?? ''}
                version={selectedVersion}
                title={title}
                values={injectedValues}
                recipients={recipients}
                signerRoleNames={signerRoleNames}
              />
            )}
          </div>

          {/* Footer */}
          <div className="flex justify-between border-t border-border p-6">
            {awaitingInputResult ? (
              <>
                <div />
                <button
                  type="button"
                  onClick={() => {
                    onOpenChange(false)
                    onSuccess?.(awaitingInputResult.id)
                  }}
                  className="flex items-center gap-1 rounded-none bg-foreground px-6 py-2.5 font-mono text-xs uppercase tracking-wider text-background transition-colors hover:bg-foreground/90"
                >
                  {t('signing.wizard.goToDocument', 'Go to Document')}
                  <ChevronRight className="h-3.5 w-3.5" />
                </button>
              </>
            ) : (
              <>
                <div>
                  {stepIndex > 0 && (
                    <button
                      type="button"
                      onClick={handleBack}
                      disabled={createDocument.isPending}
                      className="flex items-center gap-1 rounded-none border border-border bg-background px-4 py-2.5 font-mono text-xs uppercase tracking-wider text-muted-foreground transition-colors hover:border-foreground hover:text-foreground disabled:opacity-50"
                    >
                      <ChevronLeft className="h-3.5 w-3.5" />
                      {t('common.back', 'Back')}
                    </button>
                  )}
                </div>
                <div>
                  {step !== 'review' ? (
                    <button
                      type="button"
                      onClick={handleNext}
                      disabled={!canProceed}
                      className="flex items-center gap-1 rounded-none bg-foreground px-6 py-2.5 font-mono text-xs uppercase tracking-wider text-background transition-colors hover:bg-foreground/90 disabled:opacity-50"
                    >
                      {t('common.next', 'Next')}
                      <ChevronRight className="h-3.5 w-3.5" />
                    </button>
                  ) : (
                    <button
                      type="button"
                      onClick={handleSubmit}
                      disabled={createDocument.isPending}
                      className="flex items-center gap-2 rounded-none bg-foreground px-6 py-2.5 font-mono text-xs uppercase tracking-wider text-background transition-colors hover:bg-foreground/90 disabled:opacity-50"
                    >
                      <Send className="h-3.5 w-3.5" />
                      {createDocument.isPending
                        ? t('common.sending', 'Sending...')
                        : t('signing.wizard.submit', 'Send for Signing')}
                    </button>
                  )}
                </div>
              </>
            )}
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}
