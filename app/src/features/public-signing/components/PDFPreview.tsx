import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { pdfjs, Document, Page } from 'react-pdf'
import 'react-pdf/dist/Page/AnnotationLayer.css'
import 'react-pdf/dist/Page/TextLayer.css'
import {
  Loader2,
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  GripVertical,
  PenLine,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import {
  appendPublicLanguage,
  type PublicSigningLanguage,
} from '../public-signing-language'

import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
pdfjs.GlobalWorkerOptions.workerSrc = workerUrl

interface PDFPreviewProps {
  token: string
  documentTitle: string
  recipientName: string
  onProceed: () => void
  isLoading: boolean
  language?: PublicSigningLanguage
}

interface Position {
  x: number
  y: number
}

const DEFAULT_TOOLBAR_POSITION: Position = { x: 58, y: 145 }

export function PDFPreview({
  token,
  documentTitle,
  recipientName,
  onProceed,
  isLoading,
  language,
}: PDFPreviewProps) {
  const { t } = useTranslation()
  const [loadingPdf, setLoadingPdf] = useState(true)
  const [pdfError, setPdfError] = useState(false)
  const [numPages, setNumPages] = useState<number | null>(null)
  const [pageNumber, setPageNumber] = useState(1)
  const [containerWidth, setContainerWidth] = useState(0)
  const [toolbarPosition, setToolbarPosition] = useState<Position>(
    DEFAULT_TOOLBAR_POSITION,
  )
  const containerRef = useRef<HTMLDivElement>(null)
  const dragRef = useRef<{
    startX: number
    startY: number
    originX: number
    originY: number
  } | null>(null)

  // Build the PDF URL — react-pdf will fetch it directly.
  const basePath = (import.meta.env.VITE_BASE_PATH || '').replace(/\/$/, '')
  const pdfUrl = useMemo(
    () => `${basePath}${appendPublicLanguage(`/public/sign/${token}/pdf`, language)}`,
    [basePath, token, language],
  )

  // Measure container width for scaling.
  useEffect(() => {
    const updateWidth = () => {
      if (containerRef.current) {
        setContainerWidth(containerRef.current.offsetWidth - 32)
      }
    }
    updateWidth()
    window.addEventListener('resize', updateWidth)
    return () => window.removeEventListener('resize', updateWidth)
  }, [])

  const onDocumentLoadSuccess = useCallback(
    ({ numPages }: { numPages: number }) => {
      setNumPages(numPages)
      setPageNumber(1)
      setLoadingPdf(false)
    },
    [],
  )

  const goToPrevPage = useCallback(() => {
    setPageNumber((prev) => Math.max(1, prev - 1))
  }, [])

  const goToNextPage = useCallback(() => {
    setPageNumber((prev) => Math.min(numPages || prev, prev + 1))
  }, [numPages])

  const clampToolbarPosition = useCallback((next: Position): Position => {
    const width = 440
    const height = 58
    const padding = 16
    const maxX = Math.max(padding, window.innerWidth - width - padding)
    const maxY = Math.max(padding, window.innerHeight - height - padding)
    return {
      x: Math.min(Math.max(next.x, padding), maxX),
      y: Math.min(Math.max(next.y, padding), maxY),
    }
  }, [])

  useEffect(() => {
    const onMove = (event: PointerEvent) => {
      if (!dragRef.current) return
      const dx = event.clientX - dragRef.current.startX
      const dy = event.clientY - dragRef.current.startY
      setToolbarPosition(
        clampToolbarPosition({
          x: dragRef.current.originX - dx,
          y: dragRef.current.originY + dy,
        }),
      )
    }
    const onUp = () => {
      dragRef.current = null
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
  }, [clampToolbarPosition])

  const startToolbarDrag = useCallback(
    (event: React.PointerEvent<HTMLElement>) => {
      event.currentTarget.setPointerCapture?.(event.pointerId)
      dragRef.current = {
        startX: event.clientX,
        startY: event.clientY,
        originX: toolbarPosition.x,
        originY: toolbarPosition.y,
      }
    },
    [toolbarPosition],
  )

  if (pdfError) {
    return (
      <div className="space-y-8">
        <div className="flex flex-col items-center gap-4 py-12 rounded-lg border border-border bg-card">
          <AlertCircle size={48} className="text-destructive" />
          <p className="text-sm text-muted-foreground">
            {t('publicSigning.preview.pdfError')}
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Document info */}
      <div className="space-y-1">
        <h2 className="text-lg font-semibold text-foreground">
          {documentTitle}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('publicSigning.preview.readyToSign', { name: recipientName })}
        </p>
      </div>

      {/* PDF viewer */}
      <div
        ref={containerRef}
        className="relative overflow-auto rounded-lg border border-border bg-muted/30 p-4"
        style={{ minHeight: '400px', maxHeight: '70vh' }}
      >
        {loadingPdf && (
          <div className="absolute inset-0 flex items-center justify-center bg-background/80 z-10">
            <div className="flex flex-col items-center gap-2">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
              <p className="font-mono text-xs uppercase tracking-wider text-muted-foreground">
                {t('publicSigning.preview.pdfLoading')}
              </p>
            </div>
          </div>
        )}

        <Document
          file={pdfUrl}
          onLoadSuccess={onDocumentLoadSuccess}
          onLoadError={(error) => {
            console.error('PDF load error:', error)
            setPdfError(true)
            setLoadingPdf(false)
          }}
          loading={null}
        >
          <Page
            pageNumber={pageNumber}
            width={containerWidth || undefined}
            renderTextLayer={true}
            renderAnnotationLayer={true}
            className="mx-auto shadow-lg"
          />
        </Document>
      </div>

      <div
        data-testid="preview-floating-toolbar"
        aria-label={t('publicSigning.preview.toolbarAriaLabel')}
        className="fixed z-[60] w-[min(440px,calc(100vw-32px))] text-slate-100"
        style={{ right: toolbarPosition.x, top: toolbarPosition.y }}
      >
        <div
          onPointerDown={startToolbarDrag}
          className={cn(
            'flex h-[58px] cursor-grab touch-none items-center gap-3 rounded-[18px] border border-slate-500/30 px-3 active:cursor-grabbing',
            'bg-[#172232]/92 shadow-[0_18px_55px_rgba(0,0,0,0.42),inset_0_1px_0_rgba(255,255,255,0.06)]',
            'backdrop-blur-xl supports-[backdrop-filter]:bg-[#172232]/80',
          )}
        >
          <span
            aria-label={t('publicSigning.guidance.dragHandle')}
            className="grid h-6 w-4 place-items-center text-slate-400 transition-colors hover:text-slate-100"
          >
            <GripVertical size={18} />
          </span>

          <div
            className="flex items-center gap-2"
            onPointerDown={(event) => event.stopPropagation()}
          >
            <button
              type="button"
              onClick={goToPrevPage}
              disabled={!numPages || pageNumber === 1}
              className="rounded-full p-1.5 text-slate-300 transition-colors hover:bg-white/5 hover:text-white disabled:cursor-not-allowed disabled:opacity-35"
              aria-label={t('publicSigning.preview.previousPage')}
            >
              <ChevronLeft className="h-3.5 w-3.5" />
            </button>
            <span className="min-w-20 text-center font-mono text-[11px] uppercase tracking-wider text-slate-300">
              {numPages
                ? t('publicSigning.preview.pageOf', {
                    current: pageNumber,
                    total: numPages,
                  })
                : t('publicSigning.preview.pdfLoading')}
            </span>
            <button
              type="button"
              onClick={goToNextPage}
              disabled={!numPages || pageNumber === numPages}
              className="rounded-full p-1.5 text-slate-300 transition-colors hover:bg-white/5 hover:text-white disabled:cursor-not-allowed disabled:opacity-35"
              aria-label={t('publicSigning.preview.nextPage')}
            >
              <ChevronRight className="h-3.5 w-3.5" />
            </button>
          </div>

          <button
            type="button"
            onClick={onProceed}
            disabled={isLoading || loadingPdf}
            onPointerDown={(event) => event.stopPropagation()}
            className={cn(
              'ml-auto flex h-9 min-w-[144px] items-center justify-center gap-2 rounded-md px-3 text-xs font-semibold transition-colors',
              isLoading || loadingPdf
                ? 'cursor-not-allowed bg-slate-200/12 text-slate-300/55'
                : 'bg-emerald-400 text-slate-950 hover:bg-emerald-300',
            )}
          >
            {isLoading ? (
              <Loader2 size={14} className="animate-spin" />
            ) : (
              <PenLine size={14} />
            )}
            <span>
              {isLoading
                ? t('publicSigning.proceeding.title')
                : t('publicSigning.preview.proceedToSigning')}
            </span>
          </button>
        </div>
      </div>

      {/* Page navigation */}
      {numPages && numPages > 1 && (
        <div className="flex items-center justify-center gap-4 py-2">
          <button
            type="button"
            onClick={goToPrevPage}
            disabled={pageNumber === 1}
            className="p-2 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-30"
          >
            <ChevronLeft className="h-4 w-4" />
          </button>
          <span className="font-mono text-xs uppercase tracking-wider text-muted-foreground">
            {t('publicSigning.preview.pageOf', {
              current: pageNumber,
              total: numPages,
            })}
          </span>
          <button
            type="button"
            onClick={goToNextPage}
            disabled={pageNumber === numPages}
            className="p-2 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-30"
          >
            <ChevronRight className="h-4 w-4" />
          </button>
        </div>
      )}
    </div>
  )
}
