import { render, screen, waitFor, cleanup } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { PublicSigningPage } from './components/PublicSigningPage'
import { PublicDocumentAccessPage } from './components/PublicDocumentAccessPage'
import { EmbeddedModeContext } from './public-signing-embed'
import { resolveEmbeddedMode, notifyParentSigningEvent } from './public-signing-embed'
import * as publicApi from './api/public-signing-api'

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

vi.mock('./components/PDFPreview', () => ({
  PDFPreview: () => <div data-testid="pdf-preview" />,
}))

vi.mock('./api/public-signing-api', () => ({
  getPublicSigningPage: vi.fn(),
  submitPreSigningForm: vi.fn(),
  proceedToSigning: vi.fn(),
  completeEmbeddedSigning: vi.fn(),
  getDocumentAccessInfo: vi.fn(),
  requestDocumentAccess: vi.fn(),
  requestDocumentAccessFromToken: vi.fn(),
}))

const completedResponse = {
  step: 'completed' as const,
  documentTitle: 'Compromiso de Matrícula',
  recipientName: 'Adela Madrid Ávila',
  hasCurrentUserSigned: true,
  canSign: false,
  canDownload: false,
}

function stubParentWindow() {
  const postMessage = vi.fn()
  const originalParent = window.parent
  Object.defineProperty(window, 'parent', {
    configurable: true,
    value: { postMessage },
  })
  return {
    postMessage,
    restore: () => {
      Object.defineProperty(window, 'parent', {
        configurable: true,
        value: originalParent,
      })
    },
  }
}

describe('resolveEmbeddedMode', () => {
  it.each([
    [{}, false],
    [{ embedded: undefined }, false],
    [{ embedded: '0' }, false],
    [{ embedded: 'no' }, false],
    [{ embedded: false }, false],
    [{ embedded: 0 }, false],
    // TanStack Router JSON-parses search params: ?embedded=1 arrives as number 1.
    [{ embedded: 1 }, true],
    [{ embedded: '1' }, true],
    [{ embedded: 'true' }, true],
    [{ embedded: true }, true],
  ])('resolves %j to %s', (search, expected) => {
    expect(resolveEmbeddedMode(search)).toBe(expected)
  })
})

describe('notifyParentSigningEvent', () => {
  it('does nothing when not embedded in a parent window', () => {
    // In the test environment window.parent === window.
    expect(() => notifyParentSigningEvent('embed.form.completed')).not.toThrow()
  })

  it('posts the event to the parent window', () => {
    const parent = stubParentWindow()
    try {
      notifyParentSigningEvent('embed.form.exception', { reason: 'declined' })
      expect(parent.postMessage).toHaveBeenCalledWith(
        { type: 'embed.form.exception', data: { reason: 'declined' } },
        '*',
      )
    } finally {
      parent.restore()
    }
  })
})

describe('PublicSigningPage embedded mode', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(publicApi.getPublicSigningPage).mockResolvedValue(completedResponse)
  })

  afterEach(() => {
    cleanup()
    delete document.documentElement.dataset.themeLock
    document.documentElement.classList.remove('dark')
  })

  it('renders the Doc-Assembly chrome when not embedded', async () => {
    render(<PublicSigningPage token="tok" />)

    expect(await screen.findByText('publicSigning.completed.title')).toBeTruthy()
    expect(screen.getAllByText('Doc-Assembly').length).toBeGreaterThan(0)
    expect(document.documentElement.dataset.themeLock).toBeUndefined()
  })

  it('hides chrome, locks light theme and notifies the parent when embedded', async () => {
    const parent = stubParentWindow()
    document.documentElement.classList.add('dark')

    try {
      render(<PublicSigningPage token="tok" embedded />)

      expect(await screen.findByText('publicSigning.completed.title')).toBeTruthy()

      // Chrome-less: no wordmark, no language/theme toggles.
      expect(screen.queryByText('Doc-Assembly')).toBeNull()
      expect(screen.queryByTestId('language-selector')).toBeNull()
      expect(screen.queryByTestId('theme-toggle')).toBeNull()

      // Light theme pinned regardless of prior dark state.
      expect(document.documentElement.dataset.themeLock).toBe('light')
      expect(document.documentElement.classList.contains('dark')).toBe(false)

      // Host notified exactly once about the terminal state.
      await waitFor(() =>
        expect(parent.postMessage).toHaveBeenCalledWith(
          { type: 'embed.form.completed', data: undefined },
          '*',
        ),
      )
      expect(parent.postMessage).toHaveBeenCalledTimes(1)
    } finally {
      parent.restore()
    }
  })

  it('notifies completed when the current signer finished and other recipients are pending', async () => {
    const parent = stubParentWindow()
    vi.mocked(publicApi.getPublicSigningPage).mockResolvedValue({
      ...completedResponse,
      step: 'waiting' as const,
      canDownload: false,
      signingPosition: 1,
      totalSigners: 2,
      waitingForPrevious: false,
    })

    try {
      render(<PublicSigningPage token="tok" embedded />)
      // Per-recipient completion: this signer is done even though the
      // document as a whole is still waiting for others.
      await waitFor(() =>
        expect(parent.postMessage).toHaveBeenCalledWith(
          { type: 'embed.form.completed', data: undefined },
          '*',
        ),
      )
      expect(parent.postMessage).toHaveBeenCalledTimes(1)
    } finally {
      parent.restore()
    }
  })

  it('notifies the parent with an exception when the document was declined', async () => {
    const parent = stubParentWindow()
    vi.mocked(publicApi.getPublicSigningPage).mockResolvedValue({
      ...completedResponse,
      step: 'declined' as const,
      hasCurrentUserSigned: false,
    })

    try {
      render(<PublicSigningPage token="tok" embedded />)

      expect(await screen.findByText('publicSigning.declined.title')).toBeTruthy()
      await waitFor(() =>
        expect(parent.postMessage).toHaveBeenCalledWith(
          { type: 'embed.form.exception', data: { reason: 'declined' } },
          '*',
        ),
      )
    } finally {
      parent.restore()
    }
  })

  it('does not notify any parent when not embedded', async () => {
    const parent = stubParentWindow()
    try {
      render(<PublicSigningPage token="tok" />)
      expect(await screen.findByText('publicSigning.completed.title')).toBeTruthy()
      expect(parent.postMessage).not.toHaveBeenCalled()
    } finally {
      parent.restore()
    }
  })
})

describe('PublicDocumentAccessPage embedded mode', () => {
  afterEach(() => {
    cleanup()
  })

  it('renders its own chrome standalone', () => {
    render(<PublicDocumentAccessPage expiredToken="tok" expiredMessage="expired" />)
    expect(screen.getAllByText('Doc-Assembly').length).toBeGreaterThan(0)
  })

  it('hides chrome when reached from an embedded signing page', () => {
    render(
      <EmbeddedModeContext.Provider value={true}>
        <PublicDocumentAccessPage expiredToken="tok" expiredMessage="expired" />
      </EmbeddedModeContext.Provider>,
    )
    expect(screen.queryByText('Doc-Assembly')).toBeNull()
    expect(screen.queryByTestId('language-selector')).toBeNull()
    expect(screen.queryByTestId('theme-toggle')).toBeNull()
  })
})
