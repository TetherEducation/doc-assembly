import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { isAxiosError } from 'axios'
import { Copy, Check } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { useToast } from '@/components/ui/use-toast'
import { signingApi } from '../api/signing-api'
import { SigningDocumentStatus } from '../types'
import type { SigningRecipient, RecipientStatus } from '../types'

// The provider holds the envelope, so each signer has a link of their own.
const LINK_STATUSES: string[] = [
  SigningDocumentStatus.READY_TO_SIGN,
  SigningDocumentStatus.SIGNING,
]

// A signer who has signed or declined has no use for a link.
const FINISHED_RECIPIENT_STATUSES: string[] = ['SIGNED', 'DECLINED']

const RECIPIENT_STATUS_CONFIG: Record<
  string,
  { label: string; className: string }
> = {
  PENDING: {
    label: 'Pending',
    className: 'bg-muted text-muted-foreground',
  },
  SENT: {
    label: 'Sent',
    className: 'bg-yellow-500/10 text-yellow-600 dark:text-yellow-400',
  },
  DELIVERED: {
    label: 'Delivered',
    className: 'bg-blue-500/10 text-blue-600 dark:text-blue-400',
  },
  SIGNED: {
    label: 'Signed',
    className: 'bg-green-500/10 text-green-600 dark:text-green-400',
  },
  DECLINED: {
    label: 'Declined',
    className: 'bg-red-500/10 text-red-600 dark:text-red-400',
  },
}

function RecipientStatusBadge({
  status,
}: {
  status: RecipientStatus
}) {
  const config = RECIPIENT_STATUS_CONFIG[status] ?? {
    label: status,
    className: 'bg-muted text-muted-foreground',
  }

  return (
    <Badge
      className={cn(
        'border-transparent font-mono text-[10px] uppercase tracking-wider',
        config.className,
      )}
    >
      {config.label}
    </Badge>
  )
}

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

interface RecipientTableProps {
  documentId: string
  documentStatus: string
  recipients: SigningRecipient[]
  /**
   * Whether the viewer may copy signers' links. A link lets whoever holds it
   * sign as that signer, so the API requires OPERATOR or higher.
   */
  canShareLinks: boolean
}

export function RecipientTable({
  documentId,
  documentStatus,
  recipients,
  canShareLinks,
}: RecipientTableProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const [copiedId, setCopiedId] = useState<string | null>(null)

  const linksAvailable =
    canShareLinks && LINK_STATUSES.includes(documentStatus)

  const handleCopySigningURL = async (recipientId: string) => {
    try {
      const response = await signingApi.getSigningURL(documentId, recipientId)
      await navigator.clipboard.writeText(response.signingUrl)
      setCopiedId(recipientId)
      toast({
        title: t(
          'signing.detail.signingLinkCopied',
          'Signing link copied. Anyone with it can sign as this signer.',
        ),
      })
      setTimeout(() => setCopiedId(null), 2000)
    } catch (error) {
      const notReady = isAxiosError(error) && error.response?.status === 409
      toast({
        variant: 'destructive',
        title: t('common.error', 'Error'),
        description: notReady
          ? t(
              'signing.detail.signingLinkNotReady',
              "This signer's link isn't ready yet. Refresh the document in a moment.",
            )
          : t('signing.detail.urlCopyError', 'Failed to copy signing URL'),
      })
    }
  }

  if (recipients.length === 0) {
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">
        {t('signing.detail.noRecipients', 'No recipients')}
      </p>
    )
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full">
        <thead>
          <tr>
            <th className="border-b border-border py-3 pl-4 pr-4 text-left font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
              {t('signing.detail.recipientName', 'Name')}
            </th>
            <th className="border-b border-border py-3 pr-4 text-left font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
              {t('signing.detail.recipientEmail', 'Email')}
            </th>
            <th className="border-b border-border py-3 pr-4 text-left font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
              {t('signing.detail.recipientRole', 'Role')}
            </th>
            <th className="border-b border-border py-3 pr-4 text-left font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
              {t('signing.detail.recipientStatus', 'Status')}
            </th>
            <th className="border-b border-border py-3 pr-4 text-left font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
              {t('signing.detail.recipientSignedAt', 'Signed At')}
            </th>
            <th className="border-b border-border py-3 pr-4 text-right font-mono text-[10px] font-medium uppercase tracking-widest text-muted-foreground">
              {t('signing.detail.recipientActions', 'Actions')}
            </th>
          </tr>
        </thead>
        <tbody>
          {recipients.map((recipient) => (
            <tr
              key={recipient.id}
              className="transition-colors hover:bg-accent"
            >
              <td className="border-b border-border py-4 pl-4 pr-4 text-sm text-foreground">
                {recipient.name}
              </td>
              <td className="border-b border-border py-4 pr-4 text-sm text-muted-foreground">
                {recipient.email}
              </td>
              <td className="border-b border-border py-4 pr-4 text-sm text-foreground">
                {recipient.roleName}
              </td>
              <td className="border-b border-border py-4 pr-4">
                <RecipientStatusBadge status={recipient.status} />
              </td>
              <td className="border-b border-border py-4 pr-4 font-mono text-sm text-muted-foreground">
                {formatDate(recipient.signedAt)}
              </td>
              <td className="border-b border-border py-4 pr-4 text-right">
                {linksAvailable &&
                  !FINISHED_RECIPIENT_STATUSES.includes(recipient.status) && (
                    <button
                      onClick={() => handleCopySigningURL(recipient.id)}
                      className="inline-flex items-center gap-1.5 rounded-none border border-border px-3 py-1.5 font-mono text-[10px] uppercase tracking-wider text-muted-foreground transition-colors hover:border-foreground hover:text-foreground"
                      title={t(
                        'signing.detail.copySigningLinkHint',
                        "Copy this signer's own signing link to send them directly",
                      )}
                    >
                      {copiedId === recipient.id ? (
                        <Check size={12} />
                      ) : (
                        <Copy size={12} />
                      )}
                      {t('signing.detail.copySigningLink', 'Copy signing link')}
                    </button>
                  )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
