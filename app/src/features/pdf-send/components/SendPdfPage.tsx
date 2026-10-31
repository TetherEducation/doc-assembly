import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { ArrowLeft, FileUp, Loader2, Send } from 'lucide-react'
import { Document, pdfjs } from 'react-pdf'

import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'

import { useToast } from '@/components/ui/use-toast'
import { usePermission } from '@/features/auth/hooks/usePermission'
import { signingKeys } from '@/features/signing/hooks/useSigningDocuments'
import { cn } from '@/lib/utils'

import { pdfSendApi } from '../api'
import { fieldLabel } from '../field-label'
import {
  FIELD_TYPES,
  MAX_PAGES,
  MAX_PDF_BYTES,
  MAX_TITLE_LENGTH,
  findProblems,
  titleFromFileName,
  toSendPdfPayload,
  type Box,
  type FieldType,
  type Signer,
} from '../model'
import { PageThumbnails } from './PageThumbnails'
import { PdfPageWithBoxes, type PageInfo } from './PdfPageWithBoxes'
import { ReviewPanel } from './ReviewPanel'
import { SignersPanel } from './SignersPanel'

pdfjs.GlobalWorkerOptions.workerSrc = workerUrl

const MAX_RENDER_WIDTH = 820
// The page strip (w-24 plus the gap) beside the pages from the md breakpoint up.
const THUMBNAIL_COLUMN_WIDTH = 96 + 24

/**
 * "Send a PDF" (tools-team-signing docs/SPEC-2.4-field-editor.md): upload a
 * PDF, add signers in order, place signature, initials, date and text boxes
 * for each, review, send. The engine builds a one-off template and creates
 * the document (POST /documents/from-pdf); the route is only offered where the
 * deployment turns on signing.pdf_send. Nothing is saved until Send.
 */
export function SendPdfPage() {
  const { workspaceId } = useParams({ from: '/workspace/$workspaceId/signing/send-pdf' })
  const { t } = useTranslation()
  const { toast } = useToast()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { hasPermission, Permission } = usePermission()

  const nextId = useRef(0)
  const newId = useCallback((prefix: string) => `${prefix}-${++nextId.current}`, [])

  const [file, setFile] = useState<File | null>(null)
  const [fileError, setFileError] = useState<string | null>(null)
  const [numPages, setNumPages] = useState(0)
  const [pageInfos, setPageInfos] = useState<(PageInfo | undefined)[]>([])
  const [title, setTitle] = useState('')
  const [signers, setSigners] = useState<Signer[]>(() => [{ id: 'signer-0', name: '', email: '' }])
  const [currentSignerId, setCurrentSignerId] = useState('signer-0')
  const [boxes, setBoxes] = useState<Box[]>([])
  const [selectedBoxId, setSelectedBoxId] = useState<string | null>(null)
  const [activeTool, setActiveTool] = useState<FieldType | null>(null)
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)
  const [renderWidth, setRenderWidth] = useState(MAX_RENDER_WIDTH)
  const pagesRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const update = () => {
      const thumbnails = window.matchMedia?.('(min-width: 768px)').matches ? THUMBNAIL_COLUMN_WIDTH : 0
      const available = (pagesRef.current?.offsetWidth ?? MAX_RENDER_WIDTH) - 48 - thumbnails
      setRenderWidth(Math.max(320, Math.min(MAX_RENDER_WIDTH, available)))
    }
    update()
    window.addEventListener('resize', update)
    return () => window.removeEventListener('resize', update)
  }, [file])

  const deleteBox = useCallback((boxId: string) => {
    setBoxes((all) => all.filter((b) => b.id !== boxId))
    setSelectedBoxId((selected) => (selected === boxId ? null : selected))
  }, [])

  // Esc cancels placing or deselects; Delete or Backspace removes the selected
  // box, unless typing in a field.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      if (target && ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)) return
      if (event.key === 'Escape') {
        setActiveTool(null)
        setSelectedBoxId(null)
      }
      if ((event.key === 'Delete' || event.key === 'Backspace') && selectedBoxId) {
        event.preventDefault()
        deleteBox(selectedBoxId)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [selectedBoxId, deleteBox])

  const pageSizes = useMemo(() => pageInfos.map((info) => info?.size ?? { width: 0, height: 0 }), [pageInfos])
  const problems = useMemo(() => findProblems(title, signers, boxes, pageSizes), [title, signers, boxes, pageSizes])
  const offsetPages = pageInfos.flatMap((info, i) => (info?.offset ? [i + 1] : []))
  const allPagesLoaded = numPages > 0 && pageInfos.filter(Boolean).length === numPages
  const tooManyPages = numPages > MAX_PAGES
  const canSend =
    !!file && allPagesLoaded && !tooManyPages && offsetPages.length === 0 && !fileError && problems.length === 0 && !sending

  if (!hasPermission(Permission.DOCUMENT_CREATE)) {
    return (
      <div className="p-12 text-sm text-muted-foreground">
        {t('pdfSend.noPermission', 'You need permission to send documents in this workspace.')}
      </div>
    )
  }

  const chooseFile = (chosen: File | undefined) => {
    setFileError(null)
    setSendError(null)
    if (!chosen) return
    if (chosen.type !== 'application/pdf' && !chosen.name.toLowerCase().endsWith('.pdf')) {
      setFileError(t('pdfSend.notPdf', 'Choose a PDF file.'))
      return
    }
    if (chosen.size > MAX_PDF_BYTES) {
      setFileError(t('pdfSend.tooLarge', 'The PDF is larger than 12 MB.'))
      return
    }
    setFile(chosen)
    setNumPages(0)
    setPageInfos([])
    setBoxes([])
    setSelectedBoxId(null)
    if (!title.trim()) setTitle(titleFromFileName(chosen.name))
  }

  const onPageLoad = (pageNumber: number, info: PageInfo) => {
    setPageInfos((infos) => {
      const next = [...infos]
      next[pageNumber - 1] = info
      return next
    })
  }

  const scrollToPage = (pageNumber: number) => {
    document.getElementById(`pdf-send-page-${pageNumber}`)?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
  }

  const addSigner = () => {
    const signer = { id: newId('signer'), name: '', email: '' }
    setSigners((all) => [...all, signer])
    setCurrentSignerId(signer.id)
  }

  const removeSigner = (signerId: string) => {
    const remaining = signers.filter((s) => s.id !== signerId)
    setSigners(remaining)
    setBoxes((all) => all.filter((b) => b.signerId !== signerId))
    if (currentSignerId === signerId && remaining[0]) setCurrentSignerId(remaining[0].id)
  }

  const moveSigner = (from: number, to: number) => {
    setSigners((all) => {
      if (to < 0 || to >= all.length || from === to) return all
      const next = [...all]
      const [moved] = next.splice(from, 1)
      next.splice(to, 0, moved)
      return next
    })
  }

  const updateSigner = (signerId: string, patch: Partial<Signer>) => {
    setSigners((all) => all.map((s) => (s.id === signerId ? { ...s, ...patch } : s)))
  }

  const send = async () => {
    if (!file || !canSend) return
    setSending(true)
    setSendError(null)
    try {
      const doc = await pdfSendApi.send(file, toSendPdfPayload(title, signers, boxes, pageSizes))
      await queryClient.invalidateQueries({ queryKey: signingKeys.all })
      toast({ title: t('pdfSend.sent', 'Sent for signature.') })
      navigate({
        to: '/workspace/$workspaceId/signing/$documentId',
        // eslint-disable-next-line @typescript-eslint/no-explicit-any -- TanStack Router type limitation
        params: { workspaceId, documentId: doc.id } as any,
      })
    } catch (error) {
      // Stay in the editor with everything placed, and show the server's reason.
      const message =
        isAxiosError(error) && typeof error.response?.data?.error === 'string'
          ? error.response.data.error
          : t('pdfSend.sendFailed', 'The document could not be sent. Please try again.')
      setSendError(message)
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="animate-page-enter flex h-full flex-1 flex-col bg-background">
      <header className="shrink-0 border-b border-border px-4 py-4 md:px-6">
        <button
          type="button"
          onClick={() =>
            navigate({
              to: '/workspace/$workspaceId/signing',
              // eslint-disable-next-line @typescript-eslint/no-explicit-any -- TanStack Router type limitation
              params: { workspaceId } as any,
            })
          }
          className="mb-2 inline-flex items-center gap-1 font-mono text-[10px] uppercase tracking-widest text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft size={12} />
          {t('signing.header', 'Signing')}
        </button>
        <h1 className="font-display text-3xl font-light tracking-tight text-foreground">{t('pdfSend.title', 'Send a PDF')}</h1>
      </header>

      <div className="flex min-h-0 flex-1 flex-col lg:flex-row">
        <aside className="w-full shrink-0 space-y-6 overflow-y-auto border-b border-border p-4 lg:w-96 lg:border-b-0 lg:border-r">
          <section className="space-y-2">
            <span className="block font-mono text-[10px] uppercase tracking-widest text-muted-foreground">{t('pdfSend.file', 'PDF')}</span>
            <label className="flex cursor-pointer items-center gap-2 border border-dashed border-border px-3 py-3 text-sm hover:border-foreground">
              <FileUp size={16} />
              <span className="truncate">{file ? file.name : t('pdfSend.chooseFile', 'Choose a PDF (up to 12 MB)')}</span>
              <input
                type="file"
                accept="application/pdf,.pdf"
                className="sr-only"
                data-testid="pdf-file-input"
                onChange={(event) => chooseFile(event.target.files?.[0])}
              />
            </label>
            {fileError && <p className="text-sm text-destructive">{fileError}</p>}
            {offsetPages.length > 0 && (
              <p className="text-sm text-destructive">
                {t(
                  'pdfSend.offsetPages',
                  'Page {{pages}} has a visible area that does not start at the corner. Print or re-save the PDF and upload it again.',
                  { pages: offsetPages.join(', ') }
                )}
              </p>
            )}
            {tooManyPages && (
              <p className="text-sm text-destructive">
                {t('pdfSend.tooManyPages', 'The PDF has more than {{max}} pages.', { max: MAX_PAGES })}
              </p>
            )}
          </section>

          <section className="space-y-2">
            <label htmlFor="pdf-send-title" className="block font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
              {t('pdfSend.documentTitle', 'Document title')}
            </label>
            <input
              id="pdf-send-title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              maxLength={MAX_TITLE_LENGTH}
              className="h-9 w-full border border-border bg-background px-2 text-sm"
            />
          </section>

          <SignersPanel
            signers={signers}
            boxes={boxes}
            currentSignerId={currentSignerId}
            onSelectSigner={setCurrentSignerId}
            onAdd={addSigner}
            onRemove={removeSigner}
            onUpdate={updateSigner}
            onMove={moveSigner}
            onJumpToBox={(box) => {
              setSelectedBoxId(box.id)
              scrollToPage(box.page)
            }}
          />

          <section className="space-y-2">
            <span className="block font-mono text-[10px] uppercase tracking-widest text-muted-foreground">
              {t('pdfSend.place', 'Place a box for the selected signer')}
            </span>
            <div className="grid grid-cols-2 gap-2">
              {FIELD_TYPES.map((type) => (
                <button
                  key={type}
                  type="button"
                  disabled={!allPagesLoaded}
                  aria-pressed={activeTool === type}
                  onClick={() => setActiveTool(activeTool === type ? null : type)}
                  className={cn(
                    'h-9 border px-2 text-sm disabled:opacity-40',
                    activeTool === type ? 'border-foreground bg-foreground text-background' : 'border-border hover:border-foreground'
                  )}
                >
                  {fieldLabel(t, type)}
                </button>
              ))}
            </div>
            {activeTool && (
              <p className="text-xs text-muted-foreground">
                {t('pdfSend.clickToPlace', 'Click a page to drop a box, or drag to draw one. Esc to cancel.')}
              </p>
            )}
          </section>

          {file && <ReviewPanel title={title} signers={signers} boxes={boxes} numPages={numPages} problems={problems} />}

          {sendError && (
            <p className="text-sm text-destructive" role="alert">
              {sendError}
            </p>
          )}

          <button
            type="button"
            onClick={send}
            disabled={!canSend}
            className="flex h-11 w-full items-center justify-center gap-2 bg-foreground text-sm font-medium tracking-wide text-background hover:bg-foreground/90 disabled:opacity-40"
          >
            {sending ? <Loader2 size={16} className="animate-spin" /> : <Send size={16} />}
            {t('pdfSend.send', 'Send for signature')}
          </button>
        </aside>

        <main ref={pagesRef} className="min-h-0 flex-1 overflow-y-auto bg-muted/40 p-6">
          {!file ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              {t('pdfSend.empty', 'Choose a PDF to start.')}
            </div>
          ) : (
            <Document
              file={file}
              onLoadSuccess={({ numPages: pages }) => setNumPages(pages)}
              onLoadError={() => setFileError(t('pdfSend.unreadable', 'This PDF could not be read.'))}
              onPassword={() => setFileError(t('pdfSend.password', 'This PDF needs a password. Remove the password and upload it again.'))}
              loading={<Loader2 className="mx-auto animate-spin text-muted-foreground" />}
            >
              <div className="flex gap-6">
                {!tooManyPages && (
                  <div className="sticky top-0 hidden max-h-full w-24 shrink-0 self-start overflow-y-auto md:block">
                    <PageThumbnails numPages={numPages} boxes={boxes} onJump={scrollToPage} />
                  </div>
                )}
                <div className="min-w-0 flex-1 space-y-6">
                  {!tooManyPages &&
                    Array.from({ length: numPages }, (_, i) => i + 1).map((pageNumber) => (
                      <PdfPageWithBoxes
                        key={pageNumber}
                        pageNumber={pageNumber}
                        renderWidth={renderWidth}
                        pageSize={pageInfos[pageNumber - 1]?.size}
                        boxes={boxes.filter((b) => b.page === pageNumber)}
                        signers={signers}
                        selectedBoxId={selectedBoxId}
                        activeTool={activeTool}
                        currentSignerId={currentSignerId}
                        newBoxId={() => newId('box')}
                        onPageLoad={onPageLoad}
                        onPlace={(box) => {
                          setBoxes((all) => [...all, box])
                          setSelectedBoxId(box.id)
                          setActiveTool(null)
                        }}
                        onSelect={setSelectedBoxId}
                        onChange={(box) => setBoxes((all) => all.map((b) => (b.id === box.id ? box : b)))}
                        onDelete={deleteBox}
                      />
                    ))}
                </div>
              </div>
            </Document>
          )}
        </main>
      </div>
    </div>
  )
}
