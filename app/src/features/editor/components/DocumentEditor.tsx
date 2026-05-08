import { useEditor, EditorContent } from '@tiptap/react'
import StarterKit from '@tiptap/starter-kit'
import { TextStyle, FontFamily, FontSize } from '@tiptap/extension-text-style'
import { Color } from '@tiptap/extension-color'
import TextAlign from '@tiptap/extension-text-align'
import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
  type DragMoveEvent,
} from '@dnd-kit/core'
import { EditorToolbar } from './EditorToolbar'
import { PageSettings } from './PageSettings'
import { SignerRolesPanel } from './SignerRolesPanel'
import { SignerRolesProvider } from '../context/SignerRolesContext'
import { InjectorExtension } from '../extensions/Injector'
import { SignatureExtension } from '../extensions/Signature'
import { ConditionalExtension } from '../extensions/Conditional'
import { MentionExtension } from '../extensions/Mentions'
import { ImageExtension, type ImageShape } from '../extensions/Image'
import { PageBreakHR } from '../extensions/PageBreak'
import { SlashCommandsExtension, slashCommandsSuggestion } from '../extensions/SlashCommands'
import { LineSpacingExtension } from '../extensions/LineSpacing'
import {
  TableExtension,
  TableRowExtension,
  TableHeaderExtension,
  TableCellExtension,
} from '../extensions/Table'
import { TableInjectorExtension } from '../extensions/TableInjector'
import { ListInjectorExtension } from '../extensions/ListInjector'
import { InteractiveFieldExtension } from '../extensions/InteractiveField'
import { StoredMarksPersistenceExtension } from '../extensions/StoredMarksPersistence'
import { ImageInsertModal, type ImageInsertResult } from './ImageInsertModal'
import { DocumentPageHeader } from './DocumentPageHeader'
import { VariableFormatDialog } from './VariableFormatDialog'
import { VariablesPanel } from './VariablesPanel'
import { VariableDragOverlay } from './VariableDragOverlay'
import { InconsistencyNavigator } from './InconsistencyNavigator'
import { TableBubbleMenu } from './TableBubbleMenu'
import { TableCornerHandle } from './TableCornerHandle'
import { hasConfigurableOptions } from '../types/injectable'
import { type Variable } from '../types'
import { usePaginationStore, useSignerRolesStore, useDocumentHeaderStore } from '../stores'
import { useVariablesPanelStore } from '../stores/variables-panel-store'
import type { VariableDragData } from '../types/drag'
import { getInjectableVariableIds } from '../types/signer-roles'
import type { Editor } from '@tiptap/core'
import {
  getDocumentEditorGridClass,
  getDocumentEditorGridTemplateColumns,
} from '../layout/document-editor-grid'
import { deriveHeaderEnabled } from '../utils/document-header'

type ActiveSurface = 'header' | 'body'

interface DocumentEditorProps {
  initialContent?: string | Record<string, unknown>
  onContentChange?: (content: string) => void
  editable?: boolean
  variables?: Variable[]
  onExport?: () => void
  onImport?: () => void
  editorRef?: React.RefObject<Editor | null>
  onEditorReady?: (editor: Editor | null) => void
  /** Called when editor is fully rendered and styles are applied */
  onFullyReady?: () => void
  /** Template ID for preview functionality */
  templateId?: string
  /** Version ID for preview functionality */
  versionId?: string
  /** Ensures the saved preview source is up to date before generating PDF */
  onBeforePreview?: () => Promise<void>
  /** Whether to render the signer roles side panel */
  showSignerRolesPanel?: boolean
}

export function DocumentEditor({
  initialContent = '<p>Comienza a escribir...</p>',
  onContentChange,
  editable = true,
  variables = [],
  onExport,
  onImport,
  editorRef,
  onEditorReady,
  onFullyReady,
  templateId,
  versionId,
  onBeforePreview,
  showSignerRolesPanel = true,
}: DocumentEditorProps) {
  // Get page config from store (for visual width and margins)
  const { pageSize, margins } = usePaginationStore()
  const headerContent = useDocumentHeaderStore((state) => state.content)
  const headerImageUrl = useDocumentHeaderStore((state) => state.imageUrl)
  const headerImageInjectableId = useDocumentHeaderStore((state) => state.imageInjectableId)
  const headerHasMeaningfulContent = useMemo(
    () =>
      deriveHeaderEnabled({
        content: headerContent,
        imageUrl: headerImageUrl,
        imageInjectableId: headerImageInjectableId,
      }),
    [headerContent, headerImageInjectableId, headerImageUrl]
  )

  const [activeSurface, setActiveSurface] = useState<ActiveSurface>('body')
  const [headerToolbarEditor, setHeaderToolbarEditor] = useState<Editor | null>(null)
  const isVariablesPanelCollapsed = useVariablesPanelStore((state) => state.isCollapsed)
  const isRolesPanelCollapsed = useSignerRolesStore((state) => state.isCollapsed)
  const roles = useSignerRolesStore((state) => state.roles)
  const updateRole = useSignerRolesStore((state) => state.updateRole)
  const activeInjectionTarget = useSignerRolesStore((state) => state.activeInjectionTarget)
  const clearActiveInjectionTarget = useSignerRolesStore(
    (state) => state.clearActiveInjectionTarget
  )

  // Ref to store current content before editor recreation (JSON preserves custom nodes)
  const contentRef = useRef<string | Record<string, unknown>>(initialContent)

  // Key for editor recreation - only recreate when page width changes
  const editorKey = useMemo(
    () => `editor-${pageSize.width}`,
    [pageSize.width]
  )

  const [imageModalOpen, setImageModalOpen] = useState(false)
  const [isEditingImage, setIsEditingImage] = useState(false)
  const [pendingImagePosition, setPendingImagePosition] = useState<number | null>(null)
  const [editingImageShape, setEditingImageShape] = useState<ImageShape>('square')
  const [editingImageData, setEditingImageData] = useState<ImageInsertResult | null>(null)
  const [headerImageModalToken, setHeaderImageModalToken] = useState(0)

  // Format dialog state
  const [formatDialogOpen, setFormatDialogOpen] = useState(false)
  const [pendingVariable, setPendingVariable] = useState<{
    variable: Variable
    position: number
  } | null>(null)

  // Drag & drop state
  const [activeDragData, setActiveDragData] = useState<VariableDragData | null>(null)
  const [dropCursorPos, setDropCursorPos] = useState<{
    top: number
    left: number
    height: number
  } | null>(null)
  const [dropPosition, setDropPosition] = useState<number | null>(null)

  // DnD sensors - require 8px movement before drag starts (allows clicks to pass)
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  )

  const gridTemplateColumns = useMemo(
    () =>
      getDocumentEditorGridTemplateColumns({
        editable,
        variablesCollapsed: isVariablesPanelCollapsed,
        rolesCollapsed: isRolesPanelCollapsed,
        showSignerRolesPanel,
      }),
    [editable, isVariablesPanelCollapsed, isRolesPanelCollapsed, showSignerRolesPanel]
  )

  const bodyTopPadding = useMemo(
    () => (headerHasMeaningfulContent ? Math.round(margins.top / 2) : margins.top),
    [headerHasMeaningfulContent, margins.top]
  )

  // eslint-disable-next-line react-hooks/refs -- Reading ref during render is intentional to preserve content on editor recreation
  const editor = useEditor({
    immediatelyRender: false,
    extensions: [
      StarterKit.configure({
        heading: {
          levels: [1, 2, 3],
        },
      }),
      TextStyle,
      Color,
      FontFamily.configure({ types: ['textStyle'] }),
      FontSize.configure({ types: ['textStyle'] }),
      StoredMarksPersistenceExtension,
      TextAlign.configure({ types: ['heading', 'paragraph', 'tableCell', 'tableHeader'] }),
      LineSpacingExtension,
      InjectorExtension,
      MentionExtension,
      SignatureExtension,
      ConditionalExtension,
      ImageExtension,
      PageBreakHR,
      SlashCommandsExtension.configure({
        suggestion: slashCommandsSuggestion,
      }),
      TableExtension.configure({ resizable: true, lastColumnResizable: false }),
      TableRowExtension,
      TableHeaderExtension,
      TableCellExtension,
      TableInjectorExtension,
      ListInjectorExtension,
      InteractiveFieldExtension,
    ],
    // Use stored content on recreation, initial content on first render
    // eslint-disable-next-line react-hooks/refs -- Intentional: preserve content across editor recreations
    content: contentRef.current,
    editable,
    onUpdate: ({ editor }) => {
      // Store JSON for editor recreation (HTML round-trip loses custom node data)
      contentRef.current = editor.getJSON()
      onContentChange?.(editor.getHTML())
    },
    onFocus: () => {
      setActiveSurface('body')
    },
    editorProps: {
      attributes: {
        class:
          'prose prose-sm dark:prose-invert max-w-none focus:outline-none min-h-[200px]',
      },
    },
  }, [editorKey]) // Recreate editor when editorKey changes

  // Store editor reference for export/import
  useEffect(() => {
    if (editor && editorRef) {
      editorRef.current = editor
    }
    // Notify parent when editor is ready
    onEditorReady?.(editor ?? null)
  }, [editor, editorRef, onEditorReady])

  const showHeaderSurface = editable || headerHasMeaningfulContent
  const toolbarEditor = activeSurface === 'header'
    ? headerToolbarEditor
    : editor

  // Reset to body when the header surface disappears (e.g. readonly empty header)
  useEffect(() => {
    if (!showHeaderSurface && activeSurface === 'header') {
      setActiveSurface('body')
    }
  }, [activeSurface, showHeaderSurface])

  const handleHeaderEditorFocus = useCallback((headerEditor: Editor) => {
    setHeaderToolbarEditor(headerEditor)
    setActiveSurface('header')
  }, [])

  const handleHeaderEditorReady = useCallback((headerEditor: Editor | null) => {
    setHeaderToolbarEditor(headerEditor)
  }, [])

  const handleActivateHeader = useCallback(() => {
    if (!editable) return
    setActiveSurface('header')
  }, [editable])

  const handleActivateBody = useCallback(() => {
    if (!editable) return
    setActiveSurface('body')
  }, [editable])

  // Notify when editor is fully rendered and styles are applied
  useEffect(() => {
    if (!editor || !onFullyReady) return

    // Wait for next frame to ensure styles are painted
    const rafId = requestAnimationFrame(() => {
      // Additional small delay to ensure all CSS transitions complete
      const timerId = setTimeout(() => {
        onFullyReady()
      }, 50)

      return () => clearTimeout(timerId)
    })

    return () => cancelAnimationFrame(rafId)
  }, [editor, onFullyReady])

  // Listen for image modal events
  useEffect(() => {
    if (!editor) return

    const handleOpenImageModal = () => {
      setPendingImagePosition(editor.state.selection.from)
      setIsEditingImage(false)
      setImageModalOpen(true)
    }

    const handleEditImage = (event: CustomEvent<{
      src: string
      shape: ImageShape
      injectableId?: string
      injectableLabel?: string
    }>) => {
      const { src, shape, injectableId, injectableLabel } = event.detail
      setEditingImageShape(shape || 'square')
      setEditingImageData({
        src,
        isBase64: false,
        shape,
        injectableId,
        injectableLabel,
      })
      setIsEditingImage(true)
      setImageModalOpen(true)
    }

    const handleSelectVariableFormat = (
      event: CustomEvent<{
        variable: Variable
        range: { from: number; to: number }
      }>
    ) => {
      const { variable, range } = event.detail

      // Delete the @mention text
      editor.chain().focus().deleteRange(range).run()

      // Save variable and position for the dialog
      setPendingVariable({
        variable,
        position: editor.state.selection.from,
      })
      setFormatDialogOpen(true)
    }

    const dom = editor.view.dom
    dom.addEventListener('editor:open-image-modal', handleOpenImageModal)
    dom.addEventListener('editor:edit-image', handleEditImage as EventListener)
    dom.addEventListener(
      'editor:select-variable-format',
      handleSelectVariableFormat as EventListener
    )

    return () => {
      dom.removeEventListener('editor:open-image-modal', handleOpenImageModal)
      dom.removeEventListener('editor:edit-image', handleEditImage as EventListener)
      dom.removeEventListener(
        'editor:select-variable-format',
        handleSelectVariableFormat as EventListener
      )
    }
  }, [editor])

  const handleOpenBodyImageModal = useCallback(() => {
    if (!editor) return

    setActiveSurface('body')
    setPendingImagePosition(editor.state.selection.from)
    setIsEditingImage(false)
    setImageModalOpen(true)
  }, [editor])

  const handleOpenHeaderImageModal = useCallback(() => {
    if (!editable) return

    setActiveSurface('header')
    setHeaderImageModalToken((token) => token + 1)
  }, [editable])

  const handleImageInsert = useCallback((result: ImageInsertResult) => {
    if (!editor) return

    const { src, shape, injectableId, injectableLabel } = result

    if (isEditingImage) {
      // Update existing image
      editor.chain().focus().updateAttributes('customImage', {
        src,
        shape,
        injectableId: injectableId || null,
        injectableLabel: injectableLabel || null,
      }).run()
    } else {
      // Insert new image
      if (pendingImagePosition !== null) {
        editor.chain().focus().setTextSelection(pendingImagePosition).run()
      }
      editor.chain().focus().setImage({
        src,
        shape,
        injectableId,
        injectableLabel,
      }).run()
    }

    setImageModalOpen(false)
    setIsEditingImage(false)
    setPendingImagePosition(null)
    setEditingImageData(null)
  }, [editor, isEditingImage, pendingImagePosition])

  const handleImageModalClose = useCallback((open: boolean) => {
    if (!open) {
      setImageModalOpen(false)
      setIsEditingImage(false)
      setPendingImagePosition(null)
      setEditingImageData(null)
    }
  }, [])

  const handleFormatSelect = useCallback(
    (format: string) => {
      if (!editor || !pendingVariable) return

      // Use type assertion to bypass TipTap type limitations
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      ;(editor.chain().focus(pendingVariable.position) as any).setInjector({
        type: pendingVariable.variable.type,
        label: pendingVariable.variable.label,
        variableId: pendingVariable.variable.variableId,
        format,
      }).run()

      // Wait for exit animation (200ms) before unmounting
      setTimeout(() => {
        setPendingVariable(null)
      }, 200)
    },
    [editor, pendingVariable]
  )

  const handleFormatCancel = useCallback(() => {
    // Wait for exit animation (200ms) before unmounting
    setTimeout(() => {
      setPendingVariable(null)
    }, 200)
  }, [])

  /**
   * Assigns a clicked variable to the currently active signer role field target.
   * Returns true when the click is consumed by role field assignment.
   */
  const assignVariableToActiveRoleTarget = useCallback(
    (data: VariableDragData): boolean => {
      if (!activeInjectionTarget) return false
      // While a role field is targeted, consume clicks that are not plain TEXT variables
      // to avoid accidental recursive role assignments or editor insertions.
      if (data.itemType !== 'variable' || data.injectorType !== 'TEXT') {
        return true
      }

      const targetRole = roles.find((role) => role.id === activeInjectionTarget.roleId)
      if (!targetRole) {
        clearActiveInjectionTarget()
        return false
      }

      const fieldType = activeInjectionTarget.fieldType
      const currentField = targetRole[fieldType]

      if (currentField.type !== 'injectable') {
        clearActiveInjectionTarget()
        return false
      }

      if (fieldType === 'email') {
        updateRole(targetRole.id, {
          email: {
            type: 'injectable',
            value: data.variableId,
          },
        })
        return true
      }

      const currentIds = getInjectableVariableIds(currentField)
      if (currentIds.includes(data.variableId)) {
        return true
      }

      const nextIds = [...currentIds, data.variableId]
      updateRole(targetRole.id, {
        name: {
          type: 'injectable',
          value: nextIds[0] ?? '',
          values: nextIds,
          separator: 'space',
        },
      })
      return true
    },
    [
      activeInjectionTarget,
      clearActiveInjectionTarget,
      roles,
      updateRole,
    ]
  )

  // --- DRAG & DROP HANDLERS ---

  /**
   * Insert a variable into the editor at the current cursor position
   * If variable has configurable options, open format dialog
   */
  const insertVariable = useCallback(
    (data: VariableDragData, position?: number) => {
      if (!editor) return

      // Determine insertion position
      const insertPos = position ?? editor.state.selection.from

      // Si es TABLE, insertar como tableInjector (block)
      if (data.injectorType === 'TABLE') {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        ;(editor.chain().focus(insertPos) as any).setTableInjector({
          variableId: data.variableId,
          label: data.label,
        }).run()
        return
      }

      // Si es LIST, insertar como listInjector (block)
      if (data.injectorType === 'LIST') {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        ;(editor.chain().focus(insertPos) as any).setListInjector({
          variableId: data.variableId,
          label: data.label,
        }).run()
        return
      }

      // Check if variable has configurable format options
      if (hasConfigurableOptions(data.formatConfig)) {
        // Open format dialog
        setPendingVariable({
          variable: {
            id: data.id,
            variableId: data.variableId,
            label: data.label,
            type: data.injectorType,
            formatConfig: data.formatConfig,
            sourceType: data.sourceType || 'EXTERNAL',
          },
          position: insertPos,
        })
        setFormatDialogOpen(true)
      } else {
        // Insert directly without format dialog
        // Use type assertion to bypass TipTap type limitations
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        ;(editor.chain().focus(insertPos) as any).setInjector({
          type: data.itemType === 'role-variable' ? 'ROLE_TEXT' : data.injectorType,
          label: data.label,
          variableId: data.variableId,
          // For role variables, add additional attributes
          ...(data.itemType === 'role-variable' && {
            isRoleVariable: true,
            roleId: data.roleId,
            roleLabel: data.roleLabel,
            propertyKey: data.propertyKey,
          }),
        }).run()
      }
    },
    [editor]
  )

  /**
   * Handle click on variable in VariablesPanel
   * Inserts variable at current cursor position
   */
  const handleVariableClick = useCallback(
    (data: VariableDragData) => {
      if (assignVariableToActiveRoleTarget(data)) {
        return
      }
      insertVariable(data)
    },
    [insertVariable, assignVariableToActiveRoleTarget]
  )

  /**
   * Handle drag start - show overlay with ghost image
   */
  const handleDragStart = useCallback((event: DragStartEvent) => {
    const data = event.active.data.current as VariableDragData
    if (data) {
      setActiveDragData(data)
    }
  }, [])

  /**
   * Handle drag move - update drop cursor position
   * Shows visual indicator of where the variable will be inserted
   */
  const handleDragMove = useCallback(
    (event: DragMoveEvent) => {
      if (!editor) return

      const { activatorEvent, delta } = event
      if (!activatorEvent) return

      // Cast to MouseEvent since we use PointerSensor
      const pointer = activatorEvent as MouseEvent

      // Calculate position in editor at pointer coordinates
      const pos = editor.view.posAtCoords({
        left: pointer.clientX + delta.x,
        top: pointer.clientY + delta.y,
      })

      if (pos) {
        // Get visual coordinates for the drop cursor
        const coords = editor.view.coordsAtPos(pos.pos)
        setDropCursorPos({
          top: coords.top,
          left: coords.left,
          height: coords.bottom - coords.top,
        })
        // Save the position where we'll insert the variable
        setDropPosition(pos.pos)
      } else {
        setDropCursorPos(null)
        setDropPosition(null)
      }
    },
    [editor]
  )

  /**
   * Handle drag end - insert variable if dropped in editor
   */
  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active } = event

      // Get the drag data before clearing state
      const data = active.data.current as VariableDragData | undefined
      const positionToInsert = dropPosition

      // Always clear active drag data and drop cursor first
      setActiveDragData(null)
      setDropCursorPos(null)
      setDropPosition(null)

      if (!data || !editor) return

      // Insert the variable at the calculated position (not current cursor position)
      if (positionToInsert !== null) {
        insertVariable(data, positionToInsert)
      } else {
        // Fallback to current cursor position if no drop position was calculated
        insertVariable(data)
      }
    },
    [editor, insertVariable, dropPosition]
  )

  if (!editor) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
      </div>
    )
  }

  return (
    <SignerRolesProvider variables={variables}>
      <DndContext
        sensors={sensors}
        onDragStart={handleDragStart}
        onDragMove={handleDragMove}
        onDragEnd={handleDragEnd}
      >
        <div
          className={getDocumentEditorGridClass(editable, showSignerRolesPanel)}
          style={{ gridTemplateColumns }}
        >
          {/* Left: Variables Panel - only show when editable */}
          {editable && (
            <VariablesPanel
              onVariableClick={handleVariableClick}
              draggingIds={activeDragData ? [activeDragData.id] : []}
              className="row-span-2 min-w-0"
            />
          )}

          {/* Center: Main Editor Area */}
          <div className="row-span-2 grid grid-rows-subgrid min-w-0">
            {/* Header with Toolbar and Settings - Toolbar only when editable */}
            <div className="flex items-center justify-between border-b border-border bg-card min-w-0">
              {editable ? (
                <EditorToolbar
                  editor={toolbarEditor ?? editor}
                  documentEditor={editor}
                  activeSurface={activeSurface}
                  onOpenImage={activeSurface === 'header' ? handleOpenHeaderImageModal : handleOpenBodyImageModal}
                  onExport={onExport}
                  onImport={onImport}
                  onBeforePreview={onBeforePreview}
                  templateId={templateId}
                  versionId={versionId}
                />
              ) : (
                <div className="flex-1" />
              )}
              <div className="flex items-center gap-1 pr-2 shrink-0">
                <PageSettings disabled={!editable} />
              </div>
            </div>

            {/* Editor Content */}
            <div className="flex-1 overflow-auto bg-background p-8 relative">
              {/* Inconsistency Navigator - floating top-right */}
              {editable && (
                <div className="sticky top-0 z-40 flex justify-end mb-2 pointer-events-none">
                  <div className="pointer-events-auto">
                    <InconsistencyNavigator editor={editor} />
                  </div>
                </div>
              )}

              <div
                className="mx-auto bg-muted shadow-lg overflow-visible"
                style={{
                  width: pageSize.width,
                  minHeight: pageSize.height,
                }}
              >
                {showHeaderSurface && (
                  <DocumentPageHeader
                    editable={editable}
                    active={activeSurface === 'header'}
                    onActivate={handleActivateHeader}
                    onTextEditorFocus={handleHeaderEditorFocus}
                    onEditorReady={handleHeaderEditorReady}
                    openImageModalToken={headerImageModalToken}
                    paddingLeft={margins.left}
                    paddingRight={margins.right}
                  />
                )}
                <div
                  onMouseDownCapture={handleActivateBody}
                  style={{
                    paddingTop: bodyTopPadding,
                    paddingBottom: margins.bottom,
                    paddingLeft: margins.left,
                    paddingRight: margins.right,
                  }}
                >
                  <EditorContent editor={editor} />
                  {editable && <TableBubbleMenu editor={editor} />}
                </div>
              </div>
              {/* Table corner handle - positioned relative to scroll container */}
              {editable && <TableCornerHandle editor={editor} />}
            </div>
          </div>

          {/* Right: Signer Roles Panel */}
          {showSignerRolesPanel && (
            <SignerRolesPanel
              variables={variables}
              editable={editable}
              className="row-span-2 min-w-0"
            />
          )}
        </div>

        {/* Drag Overlay - shows ghost image while dragging */}
        <DragOverlay zIndex={100} dropAnimation={null}>
          {activeDragData ? <VariableDragOverlay data={activeDragData} /> : null}
        </DragOverlay>

        {/* Drop Cursor Visual Indicator */}
        {dropCursorPos && (
          <div
            className="fixed z-50 pointer-events-none"
            style={{
              top: dropCursorPos.top,
              left: dropCursorPos.left - 2,
              height: dropCursorPos.height,
            }}
          >
            <div className="h-full w-[4px] bg-blue-500 rounded-full shadow-[0_0_8px_rgba(59,130,246,0.8)]" />
            <div className="absolute -top-1.5 -left-1 w-3 h-3 bg-blue-500 rounded-full shadow-sm ring-2 ring-background" />
          </div>
        )}
      </DndContext>

      <ImageInsertModal
        open={imageModalOpen}
        onOpenChange={handleImageModalClose}
        onInsert={handleImageInsert}
        initialShape={isEditingImage ? editingImageShape : 'square'}
        initialImage={isEditingImage ? editingImageData ?? undefined : undefined}
      />

      {pendingVariable && (
        <VariableFormatDialog
          variable={pendingVariable.variable}
          open={formatDialogOpen}
          onOpenChange={setFormatDialogOpen}
          onSelect={handleFormatSelect}
          onCancel={handleFormatCancel}
        />
      )}
    </SignerRolesProvider>
  )
}
