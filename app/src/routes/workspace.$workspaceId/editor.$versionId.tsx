import { createFileRoute, Link } from '@tanstack/react-router'
import { ArrowLeft, Save } from 'lucide-react'
import { redirectSystemWorkspaceProductSurface } from '@/lib/system-workspace-guard'
import { DocumentEditor } from '@/features/editor'
import { useInjectables } from '@/features/editor/hooks/useInjectables'
import { usePaginationStore, useSignerRolesStore } from '@/features/editor/stores'
import { PAGE_SIZES, DEFAULT_MARGINS } from '@/features/editor'
import { exportAndDownload, importFromFile, type ImportResult } from '@/features/editor/services'
import { ImportValidationDialog } from '@/features/editor/components/ImportValidationDialog'
import type { DocumentMeta } from '@/features/editor/types'
import { useState, useCallback, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'

export const Route = createFileRoute('/workspace/$workspaceId/editor/$versionId')({
  beforeLoad: ({ params }) => redirectSystemWorkspaceProductSurface(params),
  component: EditorPage,
})

function EditorPage() {
  const { workspaceId, versionId } = Route.useParams()
  const { t } = useTranslation()
  const [isSaving, setIsSaving] = useState(false)

  // Load variables (injectables) from the API
  const { variables } = useInjectables()

  // Import dialog state
  const [importDialogOpen, setImportDialogOpen] = useState(false)
  const [pendingImport, setPendingImport] = useState<ImportResult | null>(null)

  // Editor ref for export/import
  // eslint-disable-next-line @typescript-eslint/no-explicit-any -- Editor type from TipTap
  const editorRef = useRef<any>(null)

  // Initialize pagination store with defaults
  useEffect(() => {
    usePaginationStore.setState({
      pageSize: PAGE_SIZES.A4,
      margins: DEFAULT_MARGINS,
    })
    useSignerRolesStore.getState().reset()
  }, [])

  // Estado del contenido (preservado entre cambios de page size)
  const contentRef = useRef<string>('<p>Comienza a escribir tu documento aqui...</p>')

  const handleContentChange = useCallback((newContent: string) => {
    contentRef.current = newContent
  }, [])

  const handleSave = useCallback(async () => {
    setIsSaving(true)
    try {
      // TODO: Implement save to API
      console.log('Saving content for version:', versionId)
      console.log('Content:', contentRef.current)
      await new Promise(resolve => setTimeout(resolve, 500))
    } finally {
      setIsSaving(false)
    }
  }, [versionId])

  // Export handler
  const handleExport = useCallback(() => {
    if (!editorRef.current) return

    const editor = editorRef.current
    const paginationStore = usePaginationStore.getState()
    const signerRolesStore = useSignerRolesStore.getState()

    const stores = {
      pagination: paginationStore,
      signerRoles: signerRolesStore.roles,
      workflowConfig: signerRolesStore.workflowConfig,
    }

    const meta: DocumentMeta = {
      title: `Version ${versionId}`,
      language: 'es',
    }

    const filename = `documento-${versionId}-${Date.now()}.json`
    exportAndDownload(editor, stores, meta, filename)
  }, [versionId])

  // Import handler
  const handleImport = useCallback(async () => {
    if (!editorRef.current) return

    const editor = editorRef.current
    const paginationStore = usePaginationStore.getState()
    const signerRolesStore = useSignerRolesStore.getState()

    const stores = {
      pagination: paginationStore,
      setSignerRoles: signerRolesStore.setRoles,
      setWorkflowConfig: signerRolesStore.setWorkflowConfig,
    }

    const result = await importFromFile(
      editor,
      stores,
      variables,
      { validateReferences: true, autoMigrate: true }
    )

    if (!result) return // User cancelled

    // Show dialog if there are errors or warnings
    if (result.validation.errors.length > 0 || result.validation.warnings.length > 0) {
      setPendingImport(result)
      setImportDialogOpen(true)
      return
    }

    // Success: no errors or warnings - update content ref
    if (editor) {
      contentRef.current = editor.getHTML()
    }
  }, [variables])

  // Confirm import after validation dialog
  const handleImportConfirm = useCallback(() => {
    setImportDialogOpen(false)
    setPendingImport(null)
    // Content was already loaded by importDocument
  }, [])

  return (
    <div className="flex flex-col h-screen min-w-0 overflow-x-hidden">
      {/* Header */}
      <header className="flex min-w-0 items-center justify-between px-4 py-2 border-b bg-card">
        <div className="flex min-w-0 flex-1 items-center gap-4">
          <Link to="/workspace/$workspaceId/templates" params={{ workspaceId }}>
            <button className="rounded-none border border-border bg-background px-6 py-2.5 font-mono text-xs uppercase tracking-wider text-muted-foreground transition-colors hover:border-foreground hover:text-foreground flex items-center">
              <ArrowLeft className="mr-2 h-4 w-4" />
              {t('common.back')}
            </button>
          </Link>
          <span className="min-w-0 truncate text-sm text-muted-foreground font-mono">
            Version: {versionId}
          </span>
        </div>
        <button
          onClick={handleSave}
          disabled={isSaving}
          className="shrink-0 rounded-none bg-foreground px-6 py-2.5 font-mono text-xs uppercase tracking-wider text-background transition-colors hover:bg-foreground/90 disabled:opacity-50 flex items-center"
        >
          <Save className="mr-2 h-4 w-4" />
          {isSaving ? t('common.saving') : t('common.save')}
        </button>
      </header>

      {/* Editor */}
      <div className="flex-1 min-w-0 overflow-hidden">
        <DocumentEditor
          key="editor"
          initialContent={contentRef.current}
          onContentChange={handleContentChange}
          variables={variables}
          onExport={handleExport}
          onImport={handleImport}
          editorRef={editorRef}
        />
      </div>

      {/* Import Validation Dialog */}
      {pendingImport && (
        <ImportValidationDialog
          open={importDialogOpen}
          onOpenChange={setImportDialogOpen}
          validation={pendingImport.validation}
          onConfirm={handleImportConfirm}
        />
      )}
    </div>
  )
}
