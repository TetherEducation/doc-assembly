import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { AlertTriangle, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useToast } from '@/components/ui/use-toast'
import { useDeprecateDocument } from '../hooks/useSigningDocuments'

interface DeprecateDocumentDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  documentId: string
  documentTitle: string
  onSuccess?: () => void
}

export function DeprecateDocumentDialog({
  open,
  onOpenChange,
  documentId,
  documentTitle,
  onSuccess,
}: DeprecateDocumentDialogProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const deprecateMutation = useDeprecateDocument()
  const [reason, setReason] = useState('')
  const isLoading = deprecateMutation.isPending

  const handleDeprecate = async () => {
    try {
      await deprecateMutation.mutateAsync({
        id: documentId,
        reason: reason.trim() || undefined,
      })
      toast({
        title: t('signing.detail.deprecateSuccess', 'Document deprecated'),
      })
      setReason('')
      onOpenChange(false)
      onSuccess?.()
    } catch {
      toast({
        variant: 'destructive',
        title: t('common.error', 'Error'),
        description: t(
          'signing.detail.deprecateError',
          'Failed to deprecate document',
        ),
      })
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 font-mono text-sm uppercase tracking-widest">
            <AlertTriangle size={18} className="text-destructive" />
            {t('signing.detail.deprecateTitle', 'Deprecate Document')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'signing.detail.deprecateConfirm',
              'Are you sure you want to deprecate "{{title}}"?',
              { title: documentTitle },
            )}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 py-4">
          <div className="rounded-sm border border-destructive/30 bg-destructive/5 p-3">
            <p className="text-sm text-destructive">
              {t(
                'signing.detail.deprecateWarning',
                'The signed document will be invalidated in doc-assembly and cleanup will be attempted in the signing provider.',
              )}
            </p>
          </div>
          <label className="block space-y-2 text-sm text-foreground">
            <span>{t('signing.detail.deprecateReason', 'Reason')}</span>
            <textarea
              aria-label={t('signing.detail.deprecateReason', 'Reason')}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              className="min-h-24 w-full rounded-none border border-border bg-background p-3 text-sm text-foreground outline-none transition-colors focus:border-foreground"
              placeholder={t(
                'signing.detail.deprecateReasonPlaceholder',
                'Why is this signed document being deprecated?',
              )}
              disabled={isLoading}
            />
          </label>
        </div>

        <DialogFooter className="gap-2 sm:gap-0">
          <button
            type="button"
            onClick={() => onOpenChange(false)}
            className="rounded-none border border-border bg-background px-6 py-2.5 font-mono text-xs uppercase tracking-wider text-muted-foreground transition-colors hover:border-foreground hover:text-foreground"
            disabled={isLoading}
          >
            {t('common.close', 'Close')}
          </button>
          <button
            type="button"
            onClick={handleDeprecate}
            className="inline-flex items-center gap-2 rounded-none bg-destructive px-6 py-2.5 font-mono text-xs uppercase tracking-wider text-destructive-foreground transition-colors hover:bg-destructive/90 disabled:opacity-50"
            disabled={isLoading}
          >
            {isLoading && <Loader2 size={14} className="animate-spin" />}
            {t('signing.detail.deprecateConfirmBtn', 'Deprecate Document')}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
