import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { SigningListPage } from './SigningListPage'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (_key: string, fallback: string) => fallback }),
}))

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  useParams: () => ({ workspaceId: 'ws-1' }),
}))

vi.mock('../hooks/useSigningDocuments', () => ({
  useSigningDocuments: () => ({ data: [], isLoading: false, isError: false }),
  useSigningDocumentTypeOptions: () => ({ data: [] }),
}))

// Only the header matters here; the list's own pieces are tested on their own.
vi.mock('./CreateDocumentWizard', () => ({ CreateDocumentWizard: () => null }))
vi.mock('./DeprecateDocumentDialog', () => ({ DeprecateDocumentDialog: () => null }))
vi.mock('./BulkActionsToolbar', () => ({ BulkActionsToolbar: () => null }))
vi.mock('./SigningListToolbar', () => ({ SigningListToolbar: () => null }))

let features: { pdfSend?: boolean } | undefined
vi.mock('@/lib/use-client-features', () => ({ useClientFeatures: () => features }))

let canCreate = true
vi.mock('@/features/auth/hooks/usePermission', () => ({
  usePermission: () => ({ hasPermission: () => canCreate, Permission: { DOCUMENT_CREATE: 'document:create' } }),
}))

describe('SigningListPage: the "Send a PDF" button', () => {
  beforeEach(() => {
    features = undefined
    canCreate = true
    navigate.mockReset()
  })
  afterEach(() => cleanup())

  it('is hidden while the deployment config has not loaded or has no pdfSend', () => {
    render(<SigningListPage />)
    expect(screen.queryByText('SEND A PDF')).toBeNull()

    cleanup()
    features = {}
    render(<SigningListPage />)
    expect(screen.queryByText('SEND A PDF')).toBeNull()
  })

  it('is hidden when the deployment turns pdfSend off (admissions)', () => {
    features = { pdfSend: false }
    render(<SigningListPage />)
    expect(screen.queryByText('SEND A PDF')).toBeNull()
    expect(screen.getByText('CREATE DOCUMENT')).toBeDefined()
  })

  it('is hidden from someone who cannot send documents', () => {
    features = { pdfSend: true }
    canCreate = false
    render(<SigningListPage />)
    expect(screen.queryByText('SEND A PDF')).toBeNull()
  })

  it('opens the editor when pdfSend is on', () => {
    features = { pdfSend: true }
    render(<SigningListPage />)

    fireEvent.click(screen.getByText('SEND A PDF'))

    expect(navigate).toHaveBeenCalledWith({
      to: '/workspace/$workspaceId/signing/send-pdf',
      params: { workspaceId: 'ws-1' },
    })
  })
})
