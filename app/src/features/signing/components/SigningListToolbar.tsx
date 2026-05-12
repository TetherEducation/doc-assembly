import { useState, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Search, ChevronDown, Check, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { SigningDocumentStatus } from '../types'
import type { DocumentTypeFilterOption } from '../types'

const ALL_STATUSES: { value: string; label: string }[] = [
  { value: SigningDocumentStatus.DRAFT, label: 'Draft' },
  { value: SigningDocumentStatus.AWAITING_INPUT, label: 'Awaiting Input' },
  { value: SigningDocumentStatus.PREPARING_SIGNATURE, label: 'Processing' },
  { value: SigningDocumentStatus.READY_TO_SIGN, label: 'Ready to Sign' },
  { value: SigningDocumentStatus.SIGNING, label: 'Signing' },
  { value: SigningDocumentStatus.COMPLETED, label: 'Completed' },
  { value: SigningDocumentStatus.DECLINED, label: 'Declined' },
  { value: SigningDocumentStatus.CANCELLED, label: 'Cancelled' },
  { value: SigningDocumentStatus.INVALIDATED, label: 'Invalidated' },
  { value: SigningDocumentStatus.ERROR, label: 'Error' },
]

interface SigningListToolbarProps {
  searchQuery: string
  onSearchChange: (query: string) => void
  documentTypeOptions: DocumentTypeFilterOption[]
  selectedDocumentTypeIds: string[]
  onDocumentTypesChange: (ids: string[]) => void
  selectedStatuses: string[]
  onStatusesChange: (statuses: string[]) => void
}

export function SigningListToolbar({
  searchQuery,
  onSearchChange,
  documentTypeOptions,
  selectedDocumentTypeIds,
  onDocumentTypesChange,
  selectedStatuses,
  onStatusesChange,
}: SigningListToolbarProps) {
  const { t } = useTranslation()
  const [statusOpen, setStatusOpen] = useState(false)
  const [templateTypeOpen, setTemplateTypeOpen] = useState(false)
  const statusRef = useRef<HTMLDivElement>(null)
  const templateTypeRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      const target = event.target as Node
      if (
        statusRef.current?.contains(target) ||
        templateTypeRef.current?.contains(target)
      ) {
        return
      }
      setStatusOpen(false)
      setTemplateTypeOpen(false)
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const handleStatusToggle = (status: string) => {
    if (selectedStatuses.includes(status)) {
      onStatusesChange(selectedStatuses.filter((s) => s !== status))
    } else {
      onStatusesChange([...selectedStatuses, status])
    }
  }

  const clearStatuses = () => {
    onStatusesChange([])
    setStatusOpen(false)
  }

  const handleDocumentTypeToggle = (id: string) => {
    if (selectedDocumentTypeIds.includes(id)) {
      onDocumentTypesChange(selectedDocumentTypeIds.filter((x) => x !== id))
    } else {
      onDocumentTypesChange([...selectedDocumentTypeIds, id])
    }
  }

  const clearDocumentTypes = () => {
    onDocumentTypesChange([])
    setTemplateTypeOpen(false)
  }

  const statusLabel =
    selectedStatuses.length === 0
      ? t('signing.status.any', 'Any')
      : `${selectedStatuses.length}`

  const templateTypeLabel =
    selectedDocumentTypeIds.length === 0
      ? t('signing.templateType.any', 'All types')
      : `${selectedDocumentTypeIds.length}`

  return (
    <div className="flex shrink-0 flex-col justify-between gap-6 border-b border-border bg-background px-4 py-6 md:flex-row md:items-center md:px-6 lg:px-6">
      {/* Search */}
      <div className="group relative w-full md:max-w-md">
        <Search
          className="absolute left-0 top-1/2 -translate-y-1/2 text-muted-foreground/50 transition-colors group-focus-within:text-foreground"
          size={20}
        />
        <input
          type="text"
          placeholder={t(
            'signing.searchPlaceholder',
            'Search by document title or signer email...',
          )}
          value={searchQuery}
          onChange={(e) => onSearchChange(e.target.value)}
          className="w-full rounded-none border-0 border-b border-border bg-transparent py-2 pl-8 pr-4 text-base font-light text-foreground outline-none transition-all placeholder:text-muted-foreground/50 focus-visible:border-foreground focus-visible:ring-0"
        />
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-6">
        {/* Template type multi-filter */}
        <div ref={templateTypeRef} className="relative">
          <button
            type="button"
            onClick={() => setTemplateTypeOpen(!templateTypeOpen)}
            className="flex items-center gap-2 font-mono text-sm uppercase tracking-wider text-muted-foreground transition-colors hover:text-foreground"
          >
            <span>
              {t('signing.templateType.label', 'Template type')}:{' '}
              {templateTypeLabel}
            </span>
            <ChevronDown
              size={16}
              className={cn(
                'transition-transform',
                templateTypeOpen && 'rotate-180',
              )}
            />
          </button>
          {templateTypeOpen && (
            <div className="absolute right-0 top-full z-50 mt-2 max-h-[300px] min-w-[220px] overflow-y-auto border border-border bg-background shadow-lg">
              {selectedDocumentTypeIds.length > 0 && (
                <button
                  type="button"
                  onClick={clearDocumentTypes}
                  className="flex w-full items-center gap-2 border-b border-border px-4 py-2 text-left text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <X size={14} />
                  <span>{t('common.clear', 'Clear all')}</span>
                </button>
              )}
              {documentTypeOptions.length === 0 ? (
                <div className="px-4 py-3 text-sm text-muted-foreground">
                  {t(
                    'signing.templateType.empty',
                    'No template types in this workspace yet.',
                  )}
                </div>
              ) : (
                documentTypeOptions.map((opt) => (
                  <button
                    type="button"
                    key={opt.id}
                    onClick={() => handleDocumentTypeToggle(opt.id)}
                    className={cn(
                      'flex w-full items-center justify-between px-4 py-2 text-left font-mono text-sm uppercase tracking-wider transition-colors hover:bg-muted',
                      selectedDocumentTypeIds.includes(opt.id)
                        ? 'text-foreground'
                        : 'text-muted-foreground',
                    )}
                  >
                    <span className="truncate pr-2">{opt.name || opt.id}</span>
                    {selectedDocumentTypeIds.includes(opt.id) && (
                      <Check size={14} className="shrink-0" />
                    )}
                  </button>
                ))
              )}
            </div>
          )}
        </div>

        {/* Status multi-filter */}
        <div ref={statusRef} className="relative">
          <button
            type="button"
            onClick={() => setStatusOpen(!statusOpen)}
            className="flex items-center gap-2 font-mono text-sm uppercase tracking-wider text-muted-foreground transition-colors hover:text-foreground"
          >
            <span>
              {t('signing.status.label', 'Status')}: {statusLabel}
            </span>
            <ChevronDown
              size={16}
              className={cn(
                'transition-transform',
                statusOpen && 'rotate-180',
              )}
            />
          </button>
          {statusOpen && (
            <div className="absolute right-0 top-full z-50 mt-2 max-h-[300px] min-w-[200px] overflow-y-auto border border-border bg-background shadow-lg">
              {selectedStatuses.length > 0 && (
                <button
                  type="button"
                  onClick={clearStatuses}
                  className="flex w-full items-center gap-2 border-b border-border px-4 py-2 text-left text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <X size={14} />
                  <span>{t('common.clear', 'Clear all')}</span>
                </button>
              )}
              {ALL_STATUSES.map((opt) => (
                <button
                  type="button"
                  key={opt.value}
                  onClick={() => handleStatusToggle(opt.value)}
                  className={cn(
                    'flex w-full items-center justify-between px-4 py-2 text-left font-mono text-sm uppercase tracking-wider transition-colors hover:bg-muted',
                    selectedStatuses.includes(opt.value)
                      ? 'text-foreground'
                      : 'text-muted-foreground',
                  )}
                >
                  <span>{opt.label}</span>
                  {selectedStatuses.includes(opt.value) && <Check size={14} />}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
