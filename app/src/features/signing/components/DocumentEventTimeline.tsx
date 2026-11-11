import { useTranslation } from 'react-i18next'
import {
  FileText,
  Send,
  CheckCircle2,
  XCircle,
  Ban,
  Clock,
  RefreshCw,
  AlertTriangle,
  Activity,
  Eye,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useDocumentEvents } from '../hooks/useDocumentEvents'
import { eventRecipient } from './eventRecipient'

const EVENT_CONFIG: Record<
  string,
  { icon: React.ElementType; label: string; colorClass: string }
> = {
  DOCUMENT_CREATED: {
    icon: FileText,
    label: 'Document created',
    colorClass: 'text-blue-500',
  },
  DOCUMENT_SENT: {
    icon: Send,
    label: 'Document sent to provider',
    colorClass: 'text-blue-500',
  },
  RECIPIENT_SENT: {
    icon: Send,
    label: 'Sent to recipient',
    colorClass: 'text-yellow-500',
  },
  RECIPIENT_DELIVERED: {
    // The provider's DELIVERED is the signer opening the document.
    icon: Eye,
    label: 'Recipient viewed',
    colorClass: 'text-blue-500',
  },
  RECIPIENT_SIGNED: {
    icon: CheckCircle2,
    label: 'Recipient signed',
    colorClass: 'text-green-500',
  },
  RECIPIENT_DECLINED: {
    icon: XCircle,
    label: 'Recipient declined',
    colorClass: 'text-red-500',
  },
  DOCUMENT_COMPLETED: {
    icon: CheckCircle2,
    label: 'Document completed',
    colorClass: 'text-green-500',
  },
  DOCUMENT_DECLINED: {
    icon: XCircle,
    label: 'Document declined',
    colorClass: 'text-red-500',
  },
  DOCUMENT_VOIDED: {
    icon: Ban,
    label: 'Document voided',
    colorClass: 'text-muted-foreground',
  },
  DOCUMENT_EXPIRED: {
    icon: Clock,
    label: 'Document expired',
    colorClass: 'text-orange-500',
  },
  DOCUMENT_REFRESHED: {
    icon: RefreshCw,
    label: 'Document status refreshed',
    colorClass: 'text-blue-500',
  },
  DOCUMENT_ERROR: {
    icon: AlertTriangle,
    label: 'Error occurred',
    colorClass: 'text-red-500',
  },
  STATUS_CHANGED: {
    icon: Activity,
    label: 'Status changed',
    colorClass: 'text-blue-500',
  },
}

function formatTimestamp(dateString: string): string {
  return new Date(dateString).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

interface DocumentEventTimelineProps {
  documentId: string
}

export function DocumentEventTimeline({
  documentId,
}: DocumentEventTimelineProps) {
  const { t } = useTranslation()
  const { data: events, isLoading } = useDocumentEvents(documentId)

  if (isLoading) {
    return (
      <div className="space-y-4 py-4">
        {[1, 2, 3].map((i) => (
          <div key={i} className="flex gap-3">
            <div className="h-5 w-5 animate-pulse rounded-full bg-muted" />
            <div className="flex-1 space-y-2">
              <div className="h-4 w-48 animate-pulse rounded bg-muted" />
              <div className="h-3 w-32 animate-pulse rounded bg-muted" />
            </div>
          </div>
        ))}
      </div>
    )
  }

  if (!events || events.length === 0) {
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">
        {t('signing.detail.noEvents', 'No events recorded')}
      </p>
    )
  }

  const sortedEvents = [...events].sort(
    (a, b) =>
      new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
  )

  return (
    <div className="relative py-2">
      {sortedEvents.map((event, index) => {
        const config = EVENT_CONFIG[event.eventType] ?? {
          icon: Activity,
          label: event.eventType.replace(/_/g, ' ').toLowerCase(),
          colorClass: 'text-muted-foreground',
        }
        const Icon = config.icon
        const recipient = event.eventType.startsWith('RECIPIENT_')
          ? eventRecipient(event.metadata)
          : null
        const isLast = index === sortedEvents.length - 1

        return (
          <div key={event.id} className="relative flex gap-3 pb-6 last:pb-0">
            {/* Vertical line */}
            {!isLast && (
              <div className="absolute left-[9px] top-6 h-full w-px bg-border" />
            )}

            {/* Icon dot */}
            <div
              className={cn(
                'relative z-10 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-background',
                config.colorClass,
              )}
            >
              <Icon size={14} />
            </div>

            {/* Content */}
            <div className="flex-1 pt-0.5">
              <p className="text-sm text-foreground">
                {config.label}
                {recipient && (
                  <span className="font-medium">
                    {' '}
                    &middot; {recipient}
                  </span>
                )}
                {event.oldStatus && event.newStatus && (
                  <span className="ml-1 text-muted-foreground">
                    ({event.oldStatus} &rarr; {event.newStatus})
                  </span>
                )}
              </p>
              <div className="mt-1 flex items-center gap-2">
                <span className="font-mono text-xs text-muted-foreground">
                  {formatTimestamp(event.createdAt)}
                </span>
                {event.actorType && (
                  <span className="rounded-sm border px-1 py-0.5 font-mono text-[10px] uppercase text-muted-foreground">
                    {event.actorType}
                  </span>
                )}
              </div>
            </div>
          </div>
        )
      })}
    </div>
  )
}
