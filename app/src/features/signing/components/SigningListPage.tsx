import { useState, useEffect, useMemo, useCallback } from 'react'
import { useNavigate, useParams } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Plus } from 'lucide-react'
import { Checkbox } from '@/components/ui/checkbox'
import { SigningListToolbar } from './SigningListToolbar'
import { SigningDocumentRow } from './SigningDocumentRow'
import { BulkActionsToolbar } from './BulkActionsToolbar'
import { DeprecateDocumentDialog } from './DeprecateDocumentDialog'
import { useSigningDocuments } from '../hooks/useSigningDocuments'
import { Skeleton } from '@/components/ui/skeleton'
import type { DocumentListFilters, SigningDocumentListItem } from '../types'

export function SigningListPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { workspaceId } = useParams({ strict: false })

  // Search with debounce
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')

  // Status multi-filter
  const [selectedStatuses, setSelectedStatuses] = useState<string[]>([])

  // Row selection
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [deprecateTarget, setDeprecateTarget] =
    useState<SigningDocumentListItem | null>(null)

  // Debounce search — clear selection when debounced value changes
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchQuery)
      setSelectedIds(new Set())
    }, 300)
    return () => clearTimeout(timer)
  }, [searchQuery])

  // Wrap status change to also clear selection
  const handleStatusesChange = useCallback((statuses: string[]) => {
    setSelectedStatuses(statuses)
    setSelectedIds(new Set())
  }, [])

  // Build filters
  const filters: DocumentListFilters = useMemo(
    () => ({
      search: debouncedSearch || undefined,
      status:
        selectedStatuses.length > 0 ? selectedStatuses.join(',') : undefined,
    }),
    [debouncedSearch, selectedStatuses],
  )

  const { data: documents, isLoading, isError } = useSigningDocuments(filters)

  const total = documents?.length ?? 0
  const hasSelection = selectedIds.size > 0
  const allSelected = total > 0 && selectedIds.size === total

  const selectedDocuments: SigningDocumentListItem[] = useMemo(
    () => documents?.filter((doc) => selectedIds.has(doc.id)) ?? [],
    [documents, selectedIds],
  )

  const handleToggleSelect = useCallback((docId: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(docId)) {
        next.delete(docId)
      } else {
        next.add(docId)
      }
      return next
    })
  }, [])

  const handleToggleSelectAll = useCallback(() => {
    if (allSelected) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(documents?.map((d) => d.id) ?? []))
    }
  }, [allSelected, documents])

  const handleClearSelection = useCallback(() => {
    setSelectedIds(new Set())
  }, [])

  const handleBulkActionComplete = useCallback(() => {
    setSelectedIds(new Set())
  }, [])

  const handleNavigateToDetail = (docId: string) => {
    navigate({
      to: '/workspace/$workspaceId/signing/$documentId',
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- TanStack Router type limitation
      params: { workspaceId: workspaceId ?? '', documentId: docId } as any,
    })
  }

  const handleNavigateToCreate = () => {
    navigate({
      to: '/workspace/$workspaceId/signing/create',
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- TanStack Router type limitation
      params: { workspaceId: workspaceId ?? '' } as any,
    })
  }

  return (
    <div className="animate-page-enter flex h-full flex-1 flex-col bg-background">
      {/* Header */}
      <header className="shrink-0 px-4 pb-6 pt-12 md:px-6 lg:px-6">
        <div className="flex flex-col justify-between gap-6 md:flex-row md:items-end">
          <div>
            <div className="mb-1 font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
              {t('signing.header', 'Signing')}
            </div>
            <h1 className="font-display text-4xl font-light leading-tight tracking-tight text-foreground md:text-5xl">
              {t('signing.title', 'Signing Documents')}
            </h1>
          </div>
          <button
            onClick={handleNavigateToCreate}
            className="group flex h-12 items-center gap-2 rounded-none bg-foreground px-6 text-sm font-medium tracking-wide text-background shadow-lg shadow-muted transition-colors hover:bg-foreground/90"
          >
            <Plus size={20} />
            <span>
              {t('signing.actions.createDocument', 'CREATE DOCUMENT')}
            </span>
          </button>
        </div>
      </header>

      {/* Toolbar */}
      <SigningListToolbar
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
        selectedStatuses={selectedStatuses}
        onStatusesChange={handleStatusesChange}
      />

      {/* Bulk Actions Toolbar */}
      {hasSelection && (
        <BulkActionsToolbar
          selectedDocuments={selectedDocuments}
          onClearSelection={handleClearSelection}
          onActionComplete={handleBulkActionComplete}
        />
      )}

      {/* Content */}
      <div className="flex-1 overflow-y-auto px-4 pb-12 md:px-6 lg:px-6">
        {/* Loading state */}
        {isLoading && (
          <div className="space-y-4 pt-6">
            {[...Array(5)].map((_, i) => (
              <Skeleton key={i} className="h-20 w-full" />
            ))}
          </div>
        )}

        {/* Error state */}
        {isError && !isLoading && (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <p className="text-lg text-muted-foreground">
              {t(
                'signing.error',
                'Failed to load signing documents. Please try again.',
              )}
            </p>
          </div>
        )}

        {/* Empty state */}
        {!isLoading && !isError && total === 0 && (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <p className="text-lg text-muted-foreground">
              {t('signing.noDocuments', 'No signing documents found')}
            </p>
            {(debouncedSearch || selectedStatuses.length > 0) && (
              <button
                onClick={() => {
                  setSearchQuery('')
                  setSelectedStatuses([])
                }}
                className="mt-4 text-sm text-foreground underline underline-offset-4 hover:no-underline"
              >
                {t('signing.clearFilters', 'Clear filters')}
              </button>
            )}
          </div>
        )}

        {/* Table */}
        {!isLoading && !isError && total > 0 && (
          <>
            <div className="py-4 font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
              {t('signing.showing', '{{count}} document(s)', {
                count: total,
              })}
            </div>
            <table className="w-full border-collapse text-left">
              <thead className="sticky top-0 z-10 bg-background">
                <tr>
                  <th className="w-[40px] border-b border-border py-4 pl-4">
                    <Checkbox
                      checked={allSelected}
                      onCheckedChange={handleToggleSelectAll}
                      aria-label={t(
                        'signing.bulk.selectAll',
                        'Select all documents',
                      )}
                    />
                  </th>
                  <th className="w-[38%] border-b border-border py-4 pl-2 font-mono text-[10px] font-normal uppercase tracking-widest text-muted-foreground">
                    {t('signing.columns.title', 'Title')}
                  </th>
                  <th className="w-[20%] border-b border-border py-4 font-mono text-[10px] font-normal uppercase tracking-widest text-muted-foreground">
                    {t('signing.columns.status', 'Status')}
                  </th>
                  <th className="w-[25%] border-b border-border py-4 font-mono text-[10px] font-normal uppercase tracking-widest text-muted-foreground">
                    {t('signing.columns.created', 'Created')}
                  </th>
                  <th className="w-[15%] border-b border-border py-4 pr-4 text-center font-mono text-[10px] font-normal uppercase tracking-widest text-muted-foreground">
                    {t('signing.columns.actions', 'Actions')}
                  </th>
                </tr>
              </thead>
              <tbody className="font-light">
                {documents?.map((doc, index) => (
                  <SigningDocumentRow
                    key={doc.id}
                    document={doc}
                    index={index}
                    selected={selectedIds.has(doc.id)}
                    onToggleSelect={() => handleToggleSelect(doc.id)}
                    onClick={() => handleNavigateToDetail(doc.id)}
                    onView={() => handleNavigateToDetail(doc.id)}
                    onDeprecate={() => setDeprecateTarget(doc)}
                  />
                ))}
              </tbody>
            </table>
          </>
        )}
      </div>

      <DeprecateDocumentDialog
        open={deprecateTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeprecateTarget(null)
        }}
        documentId={deprecateTarget?.id ?? ''}
        documentTitle={deprecateTarget?.title ?? ''}
        onSuccess={() => setDeprecateTarget(null)}
      />
    </div>
  )
}
