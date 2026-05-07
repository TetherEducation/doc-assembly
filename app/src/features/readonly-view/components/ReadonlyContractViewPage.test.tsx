import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ReadonlyContractViewPage } from './ReadonlyContractViewPage'
import * as readonlyApi from '../api/readonly-view-api'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallbackOrParams?: unknown) =>
      typeof fallbackOrParams === 'string' ? fallbackOrParams : key,
  }),
}))

vi.mock('@/components/common/LanguageSelector', () => ({
  LanguageSelector: () => <div data-testid="language-selector" />,
}))

vi.mock('@/components/common/ThemeToggle', () => ({
  ThemeToggle: () => <div data-testid="theme-toggle" />,
}))

vi.mock('@/lib/i18n', () => ({
  changeLanguage: vi.fn(() => Promise.resolve()),
}))

vi.mock('@/features/editor/components/DocumentEditor', () => ({
  DocumentEditor: vi.fn(({ initialContent, editable }) => (
    <div data-testid="document-editor" data-editable={String(editable)}>
      {typeof initialContent === 'string'
        ? initialContent
        : JSON.stringify(initialContent)}
    </div>
  )),
}))

vi.mock('./ReadonlyPdfViewer', () => ({
  ReadonlyPdfViewer: ({ pdfUrl }: { pdfUrl: string }) => (
    <div data-testid="readonly-pdf-viewer">{pdfUrl}</div>
  ),
}))

vi.mock('../api/readonly-view-api', () => ({
  getReadOnlyView: vi.fn(),
  getReadOnlyViewPdfUrl: vi.fn((token: string) => `/public/view/${token}/pdf`),
}))

describe('ReadonlyContractViewPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    cleanup()
  })

  it('renders returned content in a non-editable DocumentEditor with read-only context', async () => {
    vi.mocked(readonlyApi.getReadOnlyView).mockResolvedValue({
      mode: 'content',
      documentId: 'doc-1',
      documentTitle: 'Consulting Agreement',
      documentStatus: 'DRAFT',
      expiresAt: '2026-05-08T00:00:00Z',
      content: {
        type: 'doc',
        content: [
          { type: 'paragraph', content: [{ type: 'text', text: 'Hello client' }] },
        ],
      },
    })

    render(<ReadonlyContractViewPage token="view-token" />)

    expect(await screen.findByRole('heading', { name: 'Consulting Agreement' })).toBeTruthy()
    expect(screen.getByText('DRAFT')).toBeTruthy()
    expect(screen.getByText('Read-only view')).toBeTruthy()

    const editor = screen.getByTestId('document-editor')
    expect(editor.getAttribute('data-editable')).toBe('false')
    expect(editor.textContent).toContain('Hello client')

    await waitFor(() => {
      expect(readonlyApi.getReadOnlyView).toHaveBeenCalledWith('view-token')
    })
    expect(screen.queryByText(/sign/i)).not.toBeTruthy()
  })
})
