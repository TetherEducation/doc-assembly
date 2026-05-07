import { render, screen, waitFor, cleanup } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { PublicDocumentAccessPage } from './PublicDocumentAccessPage'
import * as publicApi from '../api/public-signing-api'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}${JSON.stringify(params)}` : key,
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

vi.mock('../api/public-signing-api', () => ({
  getDocumentAccessInfo: vi.fn(),
  requestDocumentAccess: vi.fn(),
  requestDocumentAccessFromToken: vi.fn(),
}))

describe('PublicDocumentAccessPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  afterEach(() => {
    cleanup()
  })

  it('allows requesting access for COMPLETED documents', async () => {
    vi.mocked(publicApi.getDocumentAccessInfo).mockResolvedValue({
      documentId: 'doc-1',
      documentTitle: 'Completed Contract',
      status: 'completed',
    })
    vi.mocked(publicApi.requestDocumentAccess).mockResolvedValue({
      message: 'ok',
    })

    render(<PublicDocumentAccessPage documentId="doc-1" />)

    const emailInput = await screen.findByLabelText(
      'publicSigning.access.emailLabel',
    )
    await userEvent.type(emailInput, 'alice@example.com')
    await userEvent.click(screen.getByText('publicSigning.access.sendLink'))

    await waitFor(() => {
      expect(publicApi.requestDocumentAccess).toHaveBeenCalledWith(
        'doc-1',
        'alice@example.com',
      )
    })
  })

  it('uses token endpoint when rendering expired-token mode', async () => {
    vi.mocked(publicApi.requestDocumentAccessFromToken).mockResolvedValue({
      message: 'ok',
    })

    render(
      <PublicDocumentAccessPage
        expiredToken="expired-token"
        expiredMessage="token has expired"
      />,
    )

    const emailInput = await screen.findByLabelText(
      'publicSigning.access.emailLabel',
    )
    await userEvent.type(emailInput, 'alice@example.com')
    await userEvent.click(screen.getByText('publicSigning.access.sendLink'))

    await waitFor(() => {
      expect(publicApi.requestDocumentAccessFromToken).toHaveBeenCalledWith(
        'expired-token',
        'alice@example.com',
      )
    })
    expect(publicApi.getDocumentAccessInfo).not.toHaveBeenCalled()
  })
})
