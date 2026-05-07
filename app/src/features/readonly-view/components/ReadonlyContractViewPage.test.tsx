import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ReadonlyContractViewPage } from './ReadonlyContractViewPage'
import * as readonlyApi from '../api/readonly-view-api'
import {
  useDocumentHeaderStore,
  usePaginationStore,
  useSignerRolesStore,
} from '@/features/editor/stores'

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
    usePaginationStore.getState().reset()
    useSignerRolesStore.getState().reset()
    useDocumentHeaderStore.getState().reset()
    vi.restoreAllMocks()
  })

  it('resets editor-global stores before rendering read-only content mode', async () => {
    const paginationReset = vi.spyOn(usePaginationStore.getState(), 'reset')
    const signerRolesReset = vi.spyOn(useSignerRolesStore.getState(), 'reset')
    const documentHeaderReset = vi.spyOn(useDocumentHeaderStore.getState(), 'reset')

    useSignerRolesStore.getState().setRoles([
      {
        id: 'stale-role',
        name: 'Stale Signer',
        color: '#ef4444',
        order: 1,
        fieldTypes: [],
      },
    ])
    useDocumentHeaderStore.getState().setContent({
      type: 'doc',
      content: [
        { type: 'paragraph', content: [{ type: 'text', text: 'Stale header' }] },
      ],
    })

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

    expect(await screen.findByTestId('document-editor')).toBeTruthy()

    expect(paginationReset).toHaveBeenCalledTimes(1)
    expect(signerRolesReset).toHaveBeenCalledTimes(1)
    expect(documentHeaderReset).toHaveBeenCalledTimes(1)
    expect(useSignerRolesStore.getState().roles).toEqual([])
    expect(useDocumentHeaderStore.getState().content).toBeNull()
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
