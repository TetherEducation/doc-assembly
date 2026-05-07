import { render, screen, waitFor, cleanup, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { EmbeddedSigningFrame } from './EmbeddedSigningFrame'
import * as publicApi from '../api/public-signing-api'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('../api/public-signing-api', () => ({
  getPublicSigningPage: vi.fn(),
  completeEmbeddedSigning: vi.fn(),
}))

describe('EmbeddedSigningFrame', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(publicApi.completeEmbeddedSigning).mockResolvedValue(undefined)
    vi.mocked(publicApi.getPublicSigningPage).mockResolvedValue({
      step: 'signing',
      documentTitle: 'Contract',
      recipientName: 'Signer',
      hasCurrentUserSigned: false,
      canSign: true,
      canDownload: false,
    })
  })

  afterEach(() => {
    cleanup()
  })

  it('shows finalization feedback immediately after a signing completion event while backend is not completed yet', async () => {
    const onComplete = vi.fn()

    render(
      <EmbeddedSigningFrame
        url="about:blank"
        token="public-token"
        onComplete={onComplete}
        onDecline={vi.fn()}
      />,
    )

    act(() => {
      window.dispatchEvent(
        new MessageEvent('message', {
          origin: window.location.origin,
          data: { type: 'SIGNING_EVENT', status: 'signed' },
        }),
      )
    })

    expect(
      await screen.findByText('publicSigning.finalizing.title'),
    ).toBeTruthy()
    expect(screen.getByText('publicSigning.finalizing.message')).toBeTruthy()

    await waitFor(() => {
      expect(publicApi.completeEmbeddedSigning).toHaveBeenCalledWith('public-token')
    })
    expect(onComplete).not.toHaveBeenCalled()
  })

  it('handles Documenso native document-completed iframe messages', async () => {
    const onComplete = vi.fn()

    const { container } = render(
      <EmbeddedSigningFrame
        url="about:blank"
        token="public-token"
        onComplete={onComplete}
        onDecline={vi.fn()}
      />,
    )
    const iframe = container.querySelector('iframe')
    expect(iframe?.contentWindow).toBeTruthy()

    act(() => {
      window.dispatchEvent(
        new MessageEvent('message', {
          source: iframe?.contentWindow ?? null,
          data: {
            action: 'document-completed',
            data: { token: 'provider-token', documentId: 123, recipientId: 456 },
          },
        }),
      )
    })

    expect(await screen.findByText('publicSigning.finalizing.title')).toBeTruthy()
    await waitFor(() => {
      expect(publicApi.completeEmbeddedSigning).toHaveBeenCalledWith('public-token')
    })
    expect(onComplete).not.toHaveBeenCalled()
  })

  it('keeps finalization visible for at least three seconds before showing success and completing', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(0)
    vi.mocked(publicApi.getPublicSigningPage).mockResolvedValue({
      step: 'completed',
      documentTitle: 'Contract',
      recipientName: 'Signer',
      hasCurrentUserSigned: true,
      canSign: false,
      canDownload: true,
      downloadUrl: '/download',
    })
    const onComplete = vi.fn()

    try {
      render(
        <EmbeddedSigningFrame
          url="about:blank"
          token="public-token"
          onComplete={onComplete}
          onDecline={vi.fn()}
        />,
      )

      act(() => {
        window.dispatchEvent(
          new MessageEvent('message', {
            origin: window.location.origin,
            data: { type: 'SIGNING_EVENT', status: 'signed' },
          }),
        )
      })

      expect(screen.getByText('publicSigning.finalizing.title')).toBeTruthy()

      await act(async () => {
        await vi.advanceTimersByTimeAsync(0)
      })
      expect(publicApi.completeEmbeddedSigning).toHaveBeenCalledWith('public-token')

      await act(async () => {
        await vi.advanceTimersByTimeAsync(2_999)
      })
      expect(screen.queryByText('publicSigning.finalizing.successTitle')).toBeNull()
      expect(onComplete).not.toHaveBeenCalled()

      await act(async () => {
        await vi.advanceTimersByTimeAsync(1)
      })
      expect(screen.getByText('publicSigning.finalizing.successTitle')).toBeTruthy()
      expect(onComplete).not.toHaveBeenCalled()

      await act(async () => {
        await vi.advanceTimersByTimeAsync(700)
      })
      expect(onComplete).toHaveBeenCalledTimes(1)
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not show finalization feedback for Documenso field-signed messages', async () => {
    const onComplete = vi.fn()

    const { container } = render(
      <EmbeddedSigningFrame
        url="about:blank"
        token="public-token"
        onComplete={onComplete}
        onDecline={vi.fn()}
      />,
    )
    const iframe = container.querySelector('iframe')
    expect(iframe?.contentWindow).toBeTruthy()

    act(() => {
      window.dispatchEvent(
        new MessageEvent('message', {
          source: iframe?.contentWindow ?? null,
          data: {
            action: 'field-signed',
            data: { fieldId: 123, value: 'Signer', isBase64: false },
          },
        }),
      )
    })

    expect(screen.queryByText('publicSigning.finalizing.title')).toBeNull()
    expect(publicApi.completeEmbeddedSigning).not.toHaveBeenCalled()
    expect(onComplete).not.toHaveBeenCalled()
  })

  it('ignores duplicate signing completion events while finalization is already active', async () => {
    render(
      <EmbeddedSigningFrame
        url="about:blank"
        token="public-token"
        onComplete={vi.fn()}
        onDecline={vi.fn()}
      />,
    )

    const event = new MessageEvent('message', {
      origin: window.location.origin,
      data: { type: 'SIGNING_EVENT', status: 'signed' },
    })
    act(() => {
      window.dispatchEvent(event)
      window.dispatchEvent(event)
    })

    await waitFor(() => {
      expect(publicApi.completeEmbeddedSigning).toHaveBeenCalledTimes(1)
    })
  })
})
