import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertCircle, ChevronLeft, ChevronRight, Loader2 } from 'lucide-react'
import { pdfjs, Document, Page } from 'react-pdf'
import 'react-pdf/dist/Page/AnnotationLayer.css'
import 'react-pdf/dist/Page/TextLayer.css'

import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'

pdfjs.GlobalWorkerOptions.workerSrc = workerUrl

interface ReadonlyPdfViewerProps {
  pdfUrl: string
  documentTitle: string
}

export function ReadonlyPdfViewer({
  pdfUrl,
  documentTitle,
}: ReadonlyPdfViewerProps) {
  const { t } = useTranslation()
  const [loadingPdf, setLoadingPdf] = useState(true)
  const [pdfError, setPdfError] = useState(false)
  const [numPages, setNumPages] = useState<number | null>(null)
  const [pageNumber, setPageNumber] = useState(1)
  const [containerWidth, setContainerWidth] = useState(0)
  const containerRef = useRef<HTMLDivElement>(null)

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

  if (pdfError) {
    return (
      <div className="space-y-6">
        <ReadonlyPdfHeader documentTitle={documentTitle} />
        <div className="flex flex-col items-center gap-4 rounded-lg border border-border bg-card py-12">
          <AlertCircle size={48} className="text-destructive" />
          <p className="text-sm text-muted-foreground">
            {t('readonlyView.pdf.error', 'Unable to load this PDF')}
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <ReadonlyPdfHeader documentTitle={documentTitle} />

      <div
        ref={containerRef}
        className="relative overflow-auto rounded-lg border border-border bg-muted/30 p-4"
        style={{ minHeight: '400px', maxHeight: '75vh' }}
      >
        {loadingPdf && (
          <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/80">
            <div className="flex flex-col items-center gap-2">
              <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
              <p className="font-mono text-xs uppercase tracking-wider text-muted-foreground">
                {t('readonlyView.pdf.loading', 'Loading PDF')}
              </p>
            </div>
          </div>
        )}

        <Document
          file={pdfUrl}
          onLoadSuccess={onDocumentLoadSuccess}
          onLoadError={(error) => {
            console.error('Read-only PDF load error:', error)
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

      {numPages && numPages > 0 && (
        <div className="flex items-center justify-center gap-4 py-2">
          <button
            type="button"
            onClick={goToPrevPage}
            disabled={pageNumber === 1}
            className="p-2 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-30"
            aria-label={t('readonlyView.pdf.previousPage', 'Previous page')}
          >
            <ChevronLeft className="h-4 w-4" />
          </button>
          <span className="font-mono text-xs uppercase tracking-wider text-muted-foreground">
            {t('readonlyView.pdf.pageOf', {
              current: pageNumber,
              total: numPages,
            })}
          </span>
          <button
            type="button"
            onClick={goToNextPage}
            disabled={pageNumber === numPages}
            className="p-2 text-muted-foreground transition-colors hover:text-foreground disabled:opacity-30"
            aria-label={t('readonlyView.pdf.nextPage', 'Next page')}
          >
            <ChevronRight className="h-4 w-4" />
          </button>
        </div>
      )}
    </div>
  )
}

function ReadonlyPdfHeader({ documentTitle }: { documentTitle: string }) {
  const { t } = useTranslation()
  return (
    <div className="space-y-1">
      <h2 className="text-lg font-semibold text-foreground">{documentTitle}</h2>
      <p className="font-mono text-xs uppercase tracking-wider text-muted-foreground">
        {t('readonlyView.pdf.title', 'Read-only PDF')}
      </p>
    </div>
  )
}
