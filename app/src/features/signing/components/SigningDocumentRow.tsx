import { useTranslation } from 'react-i18next'
import { motion } from 'framer-motion'
import { Ban, Eye, FileText, MoreHorizontal } from 'lucide-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Checkbox } from '@/components/ui/checkbox'
import { formatRelativeTime } from '@/lib/utils'
import { usePermission } from '@/features/auth/hooks/usePermission'
import { SigningStatusBadge } from './SigningStatusBadge'
import { SigningDocumentStatus } from '../types'
import type { SigningDocumentListItem } from '../types'

interface SigningDocumentRowProps {
  document: SigningDocumentListItem
  index?: number
  selected?: boolean
  onToggleSelect?: () => void
  onClick?: () => void
  onView?: () => void
  onDeprecate?: () => void
}

export function SigningDocumentRow({
  document,
  index = 0,
  selected = false,
  onToggleSelect,
  onClick,
  onView,
  onDeprecate,
}: SigningDocumentRowProps) {
  const MAX_VISIBLE_SIGNERS = 2
  const { t } = useTranslation()
  const { hasPermission, Permission } = usePermission()
  const documentTitle = document.title ?? t('dashboard.activity.untitled', 'Untitled')
  const templateType = document.documentTypeName ?? document.templateName
  const visibleSigners = document.recipients.slice(0, MAX_VISIBLE_SIGNERS)
  const remainingSigners = document.recipients.length - visibleSigners.length

  const shouldAnimate = index < 10
  const staggerDelay = shouldAnimate ? index * 0.05 : 0
  const canDeprecate =
    document.status === SigningDocumentStatus.COMPLETED &&
    hasPermission(Permission.DOCUMENT_DEPRECATE)

  return (
    <motion.tr
      initial={shouldAnimate ? { opacity: 0, x: 20 } : undefined}
      animate={{ opacity: 1, x: 0 }}
      transition={{
        duration: 0.2,
        ease: 'easeOut',
        delay: staggerDelay,
      }}
      onClick={onClick}
      className="group cursor-pointer transition-colors hover:bg-accent"
    >
      <td
        className="border-b border-border py-6 pl-4 align-top"
        onClick={(e) => e.stopPropagation()}
      >
        <Checkbox
          checked={selected}
          onCheckedChange={onToggleSelect}
          aria-label={t('signing.bulk.selectDocument', 'Select {{title}}', {
            title: documentTitle,
          })}
        />
      </td>
      <td className="border-b border-border py-6 pl-2 pr-4 align-top">
        <div>
          <span className="font-display text-lg font-medium text-foreground">
            {documentTitle}
          </span>
          {document.signerProvider && (
            <span className="ml-2 shrink-0 rounded-sm border px-1 py-0.5 font-mono text-[10px] uppercase text-muted-foreground">
              {document.signerProvider}
            </span>
          )}
        </div>
      </td>
      <td className="border-b border-border py-6 align-top">
        <div className="flex items-center gap-2 pt-1">
          <FileText size={14} className="shrink-0 text-muted-foreground" />
          <span className="truncate text-sm text-foreground">{templateType}</span>
        </div>
      </td>
      <td className="border-b border-border py-6 align-top">
        <div className="flex flex-wrap items-center gap-1 pt-1">
          {visibleSigners.length > 0 ? (
            <>
              {visibleSigners.map((recipient) => (
                <span
                  key={recipient.id}
                  className="inline-flex max-w-[160px] items-center rounded-full border border-border bg-muted px-2 py-0.5 text-[11px] text-foreground"
                  title={recipient.email}
                >
                  <span className="truncate">{recipient.email}</span>
                </span>
              ))}
              {remainingSigners > 0 && (
                <span className="inline-flex items-center rounded-full border border-border bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">
                  +{remainingSigners}
                </span>
              )}
            </>
          ) : (
            <span className="text-xs text-muted-foreground">
              {t('signing.noSigners', 'No signers')}
            </span>
          )}
        </div>
      </td>
      <td className="border-b border-border py-6 pt-7 align-top">
        <SigningStatusBadge status={document.status} />
      </td>
      <td className="border-b border-border py-6 pt-7 align-top font-mono text-sm text-muted-foreground">
        {formatRelativeTime(document.createdAt)}
      </td>
      <td className="border-b border-border py-6 pt-7 pr-4 text-center align-top">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              className="text-muted-foreground transition-colors hover:text-foreground"
              onClick={(e) => e.stopPropagation()}
            >
              <MoreHorizontal size={20} />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
            <DropdownMenuItem onClick={() => onView?.()}>
              <Eye className="mr-2 h-4 w-4" />
              {t('signing.actions.view', 'View details')}
            </DropdownMenuItem>
            {canDeprecate && (
              <DropdownMenuItem
                className="text-destructive focus:text-destructive"
                onClick={() => onDeprecate?.()}
              >
                <Ban className="mr-2 h-4 w-4" />
                {t('signing.actions.deprecate', 'Deprecate')}
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </td>
    </motion.tr>
  )
}
