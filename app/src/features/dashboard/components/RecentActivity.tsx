import { ArrowUpRight, Edit3, Mail, Clock, AlertTriangle, XCircle, Ban } from 'lucide-react'
import { useNavigate, useParams } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { useSigningDocuments } from '@/features/signing/hooks/useSigningDocuments'
import { Skeleton } from '@/components/ui/skeleton'
import type { SigningDocumentStatus } from '@/features/signing/types'

const statusIcons: Record<string, typeof ArrowUpRight> = {
  COMPLETED: ArrowUpRight,
  DRAFT: Edit3,
  READY_TO_SIGN: Mail,
  PREPARING_SIGNATURE: Clock,
  SIGNING: Mail,
  DECLINED: XCircle,
  CANCELLED: Ban,
  INVALIDATED: AlertTriangle,
  ERROR: AlertTriangle,
}

function formatRelativeDate(dateStr?: string): string {
  if (!dateStr) return '—'
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  if (diffHours < 1) return 'Just now'
  if (diffHours < 24) return `${diffHours}h ago`
  if (diffDays === 1) return 'Yesterday'
  if (diffDays < 7) return `${diffDays}d ago`

  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

function isActionRequired(status: SigningDocumentStatus): boolean {
  return status === 'READY_TO_SIGN' || status === 'SIGNING'
}

export function RecentActivity() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { workspaceId } = useParams({ strict: false })
  const { data: documents, isLoading } = useSigningDocuments({ pageSize: 5 })

  const handleNavigate = (docId: string) => {
    navigate({
      to: '/workspace/$workspaceId/signing/$documentId',
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- TanStack Router type limitation
      params: { workspaceId: workspaceId ?? '', documentId: docId } as any,
    })
  }

  if (isLoading) {
    return (
      <div className="w-full space-y-4">
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-16 w-full" />
        ))}
      </div>
    )
  }

  if (!documents?.length) {
    return (
      <div className="flex h-32 items-center justify-center text-sm text-muted-foreground">
        {t('dashboard.activity.empty', 'No documents yet')}
      </div>
    )
  }

  return (
    <div className="w-full">
      {/* Header */}
      <div className="grid grid-cols-12 border-b border-foreground pb-3 font-mono text-[10px] font-bold uppercase tracking-widest text-foreground">
        <div className="col-span-6 md:col-span-5">
          {t('dashboard.activity.colDocument', 'Document Name')}
        </div>
        <div className="col-span-3 md:col-span-3">
          {t('dashboard.activity.colStatus', 'Status')}
        </div>
        <div className="col-span-3 text-right md:col-span-3">
          {t('dashboard.activity.colModified', 'Modified')}
        </div>
        <div className="col-span-0 md:col-span-1" />
      </div>

      {/* Rows */}
      {documents.map((doc) => {
        const Icon = statusIcons[doc.status] ?? Edit3
        const actionReq = isActionRequired(doc.status)

        return (
          <div
            key={doc.id}
            onClick={() => handleNavigate(doc.id)}
            className="group -mx-2 grid cursor-pointer grid-cols-12 items-center border-b border-border px-2 py-5 transition-colors hover:bg-accent"
          >
            <div className="col-span-6 pr-4 md:col-span-5">
              <div className="truncate text-sm font-medium text-foreground">
                {doc.title || t('dashboard.activity.untitled', 'Untitled')}
              </div>
              <div className="mt-1 truncate font-mono text-[11px] text-muted-foreground">
                ID: {doc.id.slice(0, 8)}
              </div>
            </div>
            <div className="col-span-3 md:col-span-3">
              <span
                className={cn(
                  'inline-flex items-center rounded-none border px-2 py-1 font-mono text-[9px] font-bold uppercase tracking-wider',
                  actionReq
                    ? 'border-foreground bg-foreground text-background'
                    : 'border-border bg-background text-muted-foreground group-hover:border-foreground group-hover:text-foreground'
                )}
              >
                {t(`signing.status.${doc.status}`, doc.status)}
              </span>
            </div>
            <div className="col-span-3 text-right font-mono text-xs text-muted-foreground transition-colors group-hover:text-foreground md:col-span-3">
              {formatRelativeDate(doc.updatedAt ?? doc.createdAt)}
            </div>
            <div className="col-span-0 flex justify-end opacity-0 transition-opacity group-hover:opacity-100 md:col-span-1">
              <Icon size={16} className="text-foreground" />
            </div>
          </div>
        )
      })}
    </div>
  )
}
