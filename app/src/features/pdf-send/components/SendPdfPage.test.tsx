import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AxiosError, AxiosHeaders } from 'axios'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { pdfSendApi } from '../api'
import { SendPdfPage } from './SendPdfPage'

// One page. Tests change its frame: `view` is the visible box react-pdf reports,
// `viewport` what getViewport({ scale: 1 }) returns (CropBox and /Rotate applied).
let pageView = [0, 0, 612, 792]
let viewport = { width: 612, height: 792 }
vi.mock('react-pdf', () => ({
  pdfjs: { GlobalWorkerOptions: { workerSrc: '' } },
  Document: ({ children, onLoadSuccess }: { children: React.ReactNode; onLoadSuccess: (r: { numPages: number }) => void }) => {
    setTimeout(() => onLoadSuccess({ numPages: 1 }), 0)
    return <div data-testid="pdf-document">{children}</div>
  },
  Page: ({ pageNumber, onLoadSuccess }: { pageNumber: number; onLoadSuccess: (p: unknown) => void }) => {
    setTimeout(() => onLoadSuccess({ view: pageView, getViewport: () => viewport }), 0)
    return <div data-testid="pdf-page">page {pageNumber}</div>
  },
  Thumbnail: ({ pageNumber, onItemClick }: { pageNumber: number; onItemClick: (a: { pageNumber: number }) => void }) => (
    <button type="button" onClick={() => onItemClick({ pageNumber })}>
      thumbnail {pageNumber}
    </button>
  ),
}))

vi.mock('pdfjs-dist/build/pdf.worker.min.mjs?url', () => ({ default: '/pdf.worker.min.mjs' }))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback: string, options?: Record<string, unknown>) =>
      typeof fallback === 'string' ? fallback.replace(/\{\{(\w+)\}\}/g, (_, k: string) => String(options?.[k] ?? '')) : _key,
  }),
}))

const navigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => navigate,
  useParams: () => ({ workspaceId: 'ws-1' }),
}))

vi.mock('@/features/auth/hooks/usePermission', () => ({
  usePermission: () => ({ hasPermission: () => true, Permission: { DOCUMENT_CREATE: 'document:create' } }),
}))

vi.mock('@/components/ui/use-toast', () => ({ useToast: () => ({ toast: vi.fn() }) }))

vi.mock('../api', () => ({ pdfSendApi: { send: vi.fn() } }))

// The page is drawn 320 px wide in happy-dom (no layout), so 1 px = page width / 320 points.
const RENDER_WIDTH = 320

const renderPage = () =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <SendPdfPage />
    </QueryClientProvider>
  )

const choosePdf = async () => {
  const file = new File(['%PDF-1.4'], 'Mutual_NDA.pdf', { type: 'application/pdf' })
  fireEvent.change(screen.getByTestId('pdf-file-input'), { target: { files: [file] } })
  await screen.findByTestId('pdf-page-overlay-1')
  await waitFor(() => expect(screen.getByRole('button', { name: 'Signature' })).toHaveProperty('disabled', false))
  return file
}

const fillSigner = (n: number, name: string, email: string) => {
  fireEvent.change(screen.getByLabelText(`Signer ${n} name`), { target: { value: name } })
  fireEvent.change(screen.getByLabelText(`Signer ${n} email`), { target: { value: email } })
}

/** Picks a field type and presses then releases on the page, at CSS pixels. */
const place = async (type: string, from: [number, number], to: [number, number] = from) => {
  fireEvent.click(screen.getByRole('button', { name: type }))
  const overlay = screen.getByTestId('pdf-page-overlay-1')
  fireEvent.pointerDown(overlay, { clientX: from[0], clientY: from[1], pointerId: 1 })
  fireEvent.pointerUp(overlay, { clientX: to[0], clientY: to[1], pointerId: 1 })
  await screen.findAllByRole('button', { name: new RegExp(`^${type}:`) })
}

const sendButton = () => screen.getByRole('button', { name: 'Send for signature' })

describe('SendPdfPage', () => {
  beforeEach(() => {
    pageView = [0, 0, 612, 792]
    viewport = { width: 612, height: 792 }
    navigate.mockReset()
    vi.mocked(pdfSendApi.send).mockReset()
  })
  afterEach(() => cleanup())

  it('starts empty, with sending disabled', () => {
    renderPage()
    expect(screen.getByText('Choose a PDF to start.')).toBeDefined()
    expect(sendButton()).toHaveProperty('disabled', true)
  })

  it('refuses a file that is not a PDF', () => {
    renderPage()
    fireEvent.change(screen.getByTestId('pdf-file-input'), { target: { files: [new File(['hi'], 'notes.txt', { type: 'text/plain' })] } })
    expect(screen.getByText('Choose a PDF file.')).toBeDefined()
  })

  it('blocks Send, and says why, until every signer has a signature box', async () => {
    renderPage()
    await choosePdf()
    fillSigner(1, 'Ana', 'ana@example.test')

    expect((await screen.findByTestId('pdf-send-problems')).textContent).toContain('Ana has no signature box.')
    expect(sendButton()).toHaveProperty('disabled', true)

    await place('Date', [100, 100])
    expect(screen.getByTestId('pdf-send-problems').textContent).toContain('Ana has no signature box.')
    expect(sendButton()).toHaveProperty('disabled', true)

    await place('Signature', [160, 200])
    await waitFor(() => expect(screen.queryByTestId('pdf-send-problems')).toBeNull())
    expect(sendButton()).toHaveProperty('disabled', false)
  })

  it('sends the signer and the box in PDF points from the bottom-left, then opens the document', async () => {
    vi.mocked(pdfSendApi.send).mockResolvedValue({ id: 'doc-9' } as never)
    renderPage()
    const file = await choosePdf()
    expect(screen.getByLabelText('Document title')).toHaveProperty('value', 'Mutual NDA')
    fillSigner(1, 'Ana', 'ana@example.test')
    await place('Signature', [160, 200])

    fireEvent.click(sendButton())

    await waitFor(() => expect(pdfSendApi.send).toHaveBeenCalledTimes(1))
    const [sentFile, payload] = vi.mocked(pdfSendApi.send).mock.calls[0]
    expect(sentFile).toBe(file)
    expect(payload.title).toBe('Mutual NDA')
    expect(payload.signers).toEqual([
      expect.objectContaining({ name: 'Ana', email: 'ana@example.test', order: 1 }),
    ])
    const [field] = payload.signers[0].fields
    const s = RENDER_WIDTH / 612
    // A click drops a 180×40 signature centred on the point.
    expect(field).toMatchObject({ type: 'SIGNATURE', page: 1, width: 180, height: 40 })
    expect(field.x).toBeCloseTo(160 / s - 90, 1)
    expect(field.y).toBeCloseTo(792 - (200 / s - 20) - 40, 1)
    await waitFor(() =>
      expect(navigate).toHaveBeenCalledWith({
        to: '/workspace/$workspaceId/signing/$documentId',
        params: { workspaceId: 'ws-1', documentId: 'doc-9' },
      })
    )
  })

  it('draws a box by dragging, and the box can change type', async () => {
    vi.mocked(pdfSendApi.send).mockResolvedValue({ id: 'doc-1' } as never)
    renderPage()
    await choosePdf()
    fillSigner(1, 'Ana', 'ana@example.test')
    await place('Signature', [160, 200])
    await place('Text', [100, 100], [300, 150])
    fireEvent.change(screen.getByLabelText('Field type for this box'), { target: { value: 'DATE' } })

    fireEvent.click(sendButton())

    await waitFor(() => expect(pdfSendApi.send).toHaveBeenCalled())
    const [, payload] = vi.mocked(pdfSendApi.send).mock.calls[0]
    const drawn = payload.signers[0].fields[1]
    const s = RENDER_WIDTH / 612
    expect(drawn.type).toBe('DATE')
    expect(drawn.x).toBeCloseTo(100 / s, 1)
    expect(drawn.width).toBeCloseTo(200 / s, 1)
    expect(drawn.height).toBeCloseTo(50 / s, 1)
  })

  it('keeps everything placed and shows the server\'s reason on a 400', async () => {
    vi.mocked(pdfSendApi.send).mockRejectedValue(
      new AxiosError('bad request', 'ERR_BAD_REQUEST', undefined, undefined, {
        status: 400,
        statusText: 'Bad Request',
        data: { error: 'invalid pdf: the PDF needs a password' },
        headers: {},
        config: { headers: new AxiosHeaders() },
      })
    )
    renderPage()
    await choosePdf()
    fillSigner(1, 'Ana', 'ana@example.test')
    await place('Signature', [160, 200])

    fireEvent.click(sendButton())

    expect((await screen.findByRole('alert')).textContent).toBe('invalid pdf: the PDF needs a password')
    expect(screen.getAllByRole('button', { name: /^Signature:/ })).toHaveLength(1)
    expect(screen.getByLabelText('Signer 1 name')).toHaveProperty('value', 'Ana')
    expect(navigate).not.toHaveBeenCalled()
    await waitFor(() => expect(sendButton()).toHaveProperty('disabled', false))
  })

  it('disables Send while the request runs, so one click makes one document', async () => {
    let finish: (value: unknown) => void = () => {}
    vi.mocked(pdfSendApi.send).mockReturnValue(new Promise((resolve) => (finish = resolve)) as never)
    renderPage()
    await choosePdf()
    fillSigner(1, 'Ana', 'ana@example.test')
    await place('Signature', [160, 200])

    fireEvent.click(sendButton())
    await waitFor(() => expect(sendButton()).toHaveProperty('disabled', true))
    fireEvent.click(sendButton())
    expect(pdfSendApi.send).toHaveBeenCalledTimes(1)

    finish({ id: 'doc-2' })
    await waitFor(() => expect(navigate).toHaveBeenCalled())
  })

  it('removing a signer removes their boxes', async () => {
    renderPage()
    await choosePdf()
    fireEvent.click(screen.getByRole('button', { name: /Add signer/ }))
    await place('Signature', [160, 200])
    expect(screen.getAllByRole('button', { name: /^Signature:/ })).toHaveLength(1)

    // The box went to signer 2, the one selected when it was added.
    fireEvent.click(screen.getAllByRole('button', { name: 'Remove signer' })[1])

    expect(screen.queryAllByRole('button', { name: /^Signature:/ })).toHaveLength(0)
  })

  it('reorders signers: the payload order is the row number', async () => {
    vi.mocked(pdfSendApi.send).mockResolvedValue({ id: 'doc-3' } as never)
    renderPage()
    await choosePdf()
    fillSigner(1, 'Ana', 'ana@example.test')
    await place('Signature', [160, 200])
    fireEvent.click(screen.getByRole('button', { name: /Add signer/ }))
    fillSigner(2, 'Ben', 'ben@example.test')
    await place('Signature', [160, 400])

    fireEvent.click(screen.getAllByRole('button', { name: 'Move up' })[1])
    fireEvent.click(sendButton())

    await waitFor(() => expect(pdfSendApi.send).toHaveBeenCalled())
    const [, payload] = vi.mocked(pdfSendApi.send).mock.calls[0]
    expect(payload.signers.map((s) => [s.name, s.order])).toEqual([
      ['Ben', 1],
      ['Ana', 2],
    ])
  })

  it('jumps to a box from the signer\'s list', async () => {
    const scrollIntoView = vi.fn()
    Element.prototype.scrollIntoView = scrollIntoView
    renderPage()
    await choosePdf()
    await place('Initials', [160, 200])
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(screen.getByRole('button', { name: /^Initials:/ }).getAttribute('aria-pressed')).toBe('false')

    fireEvent.click(screen.getByRole('button', { name: 'Initials · p.1' }))

    expect(screen.getByRole('button', { name: /^Initials:/ }).getAttribute('aria-pressed')).toBe('true')
    expect(scrollIntoView).toHaveBeenCalled()
  })

  it('places boxes in the rotated page\'s frame (pdf.js viewport), not the raw page box', async () => {
    // A portrait page with /Rotate 90: pdf.js draws it landscape.
    viewport = { width: 792, height: 612 }
    vi.mocked(pdfSendApi.send).mockResolvedValue({ id: 'doc-4' } as never)
    renderPage()
    await choosePdf()
    fillSigner(1, 'Ana', 'ana@example.test')
    await place('Signature', [160, 100])

    fireEvent.click(sendButton())

    await waitFor(() => expect(pdfSendApi.send).toHaveBeenCalled())
    const [, payload] = vi.mocked(pdfSendApi.send).mock.calls[0]
    const s = RENDER_WIDTH / 792
    expect(payload.signers[0].fields[0].y).toBeCloseTo(612 - (100 / s - 20) - 40, 1)
  })

  it('refuses a page whose visible area does not start at the corner', async () => {
    pageView = [36, 36, 648, 828]
    renderPage()
    await choosePdf()

    expect(await screen.findByText(/does not start at the corner/)).toBeDefined()
    fillSigner(1, 'Ana', 'ana@example.test')
    await place('Signature', [160, 200])
    expect(sendButton()).toHaveProperty('disabled', true)
  })
})
