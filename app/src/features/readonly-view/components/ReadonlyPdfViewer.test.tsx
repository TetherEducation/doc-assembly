import { render, screen, cleanup, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { ReadonlyPdfViewer } from './ReadonlyPdfViewer'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallbackOrParams?: unknown, params?: Record<string, unknown>) => {
      if (typeof fallbackOrParams === 'string') return fallbackOrParams
      const values = (fallbackOrParams ?? params) as Record<string, unknown> | undefined
      return values ? `${key}${JSON.stringify(values)}` : key
    },
  }),
}))

vi.mock('react-pdf', () => ({
  pdfjs: { GlobalWorkerOptions: { workerSrc: '' } },
  Document: ({ children, onLoadSuccess }: { children: React.ReactNode; onLoadSuccess: (result: { numPages: number }) => void }) => {
    setTimeout(() => onLoadSuccess({ numPages: 2 }), 0)
    return <div data-testid="pdf-document">{children}</div>
  },
  Page: ({ pageNumber }: { pageNumber: number }) => (
    <div data-testid="pdf-page">page {pageNumber}</div>
  ),
}))

vi.mock('pdfjs-dist/build/pdf.worker.min.mjs?url', () => ({
  default: '/pdf.worker.min.mjs',
}))

describe('ReadonlyPdfViewer', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders PDF page controls without signing actions', async () => {
    render(
      <ReadonlyPdfViewer
        pdfUrl="/public/view/view-token/pdf"
        documentTitle="Executed Agreement"
      />,
    )

    expect(screen.getByRole('heading', { name: 'Executed Agreement' })).toBeTruthy()
    expect(screen.getByText('Read-only PDF')).toBeTruthy()
    expect(screen.getByTestId('pdf-document')).toBeTruthy()

    await waitFor(() => {
      expect(screen.getByText('readonlyView.pdf.pageOf{"current":1,"total":2}')).toBeTruthy()
    })

    expect(screen.queryByRole('button', { name: /sign/i })).not.toBeTruthy()
    expect(screen.queryByText(/proceed/i)).not.toBeTruthy()
  })
})
