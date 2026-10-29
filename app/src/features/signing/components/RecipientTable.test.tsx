import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AxiosError, AxiosHeaders } from 'axios'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { RecipientTable } from './RecipientTable'
import { signingApi } from '../api/signing-api'
import type { SigningRecipient } from '../types'

// Mock react-i18next
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback: string) => fallback,
  }),
}))

// Mock useToast
const toast = vi.fn()
vi.mock('@/components/ui/use-toast', () => ({
  useToast: () => ({ toast }),
}))

// Mock signing API
vi.mock('../api/signing-api', () => ({
  signingApi: {
    getSigningURL: vi.fn(),
  },
}))

const makeRecipient = (
  overrides: Partial<SigningRecipient> = {},
): SigningRecipient => ({
  id: 'r1',
  roleId: 'role-1',
  roleName: 'Signer',
  name: 'John Doe',
  email: 'john@example.com',
  status: 'PENDING',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  ...overrides,
})

// Defaults: a document out for signature, seen by someone who may send.
const renderTable = (
  props: Partial<React.ComponentProps<typeof RecipientTable>> = {},
) =>
  render(
    <RecipientTable
      documentId="doc-1"
      documentStatus="SIGNING"
      recipients={[makeRecipient()]}
      canShareLinks
      {...props}
    />,
  )

describe('RecipientTable', () => {
  beforeEach(() => {
    vi.mocked(signingApi.getSigningURL).mockReset()
    toast.mockReset()
  })

  it('shows empty state when no recipients', () => {
    renderTable({ recipients: [] })
    expect(screen.getByText('No recipients')).toBeDefined()
  })

  it('renders table headers', () => {
    renderTable()
    expect(screen.getByText('Name')).toBeDefined()
    expect(screen.getByText('Email')).toBeDefined()
    expect(screen.getByText('Role')).toBeDefined()
    expect(screen.getByText('Status')).toBeDefined()
    expect(screen.getByText('Signed At')).toBeDefined()
    expect(screen.getByText('Actions')).toBeDefined()
  })

  it('renders recipient name, email, and role', () => {
    renderTable({
      recipients: [
        makeRecipient({
          name: 'Alice Smith',
          email: 'alice@example.com',
          roleName: 'Approver',
        }),
      ],
    })
    expect(screen.getByText('Alice Smith')).toBeDefined()
    expect(screen.getByText('alice@example.com')).toBeDefined()
    expect(screen.getByText('Approver')).toBeDefined()
  })

  it('renders recipient status badge', () => {
    renderTable({ recipients: [makeRecipient({ status: 'SIGNED' })] })
    expect(screen.getByText('Signed')).toBeDefined()
  })

  it('renders multiple recipients', () => {
    renderTable({
      recipients: [
        makeRecipient({ id: 'r1', name: 'Alice', email: 'alice@test.com' }),
        makeRecipient({ id: 'r2', name: 'Bob', email: 'bob@test.com' }),
        makeRecipient({ id: 'r3', name: 'Charlie', email: 'charlie@test.com' }),
      ],
    })
    expect(screen.getByText('Alice')).toBeDefined()
    expect(screen.getByText('Bob')).toBeDefined()
    expect(screen.getByText('Charlie')).toBeDefined()
  })

  it('offers a signing link only to signers who still have to sign', () => {
    renderTable({
      recipients: [
        makeRecipient({ id: 'r1', name: 'Alice', status: 'SIGNED' }),
        makeRecipient({ id: 'r2', name: 'Bob', status: 'SENT' }),
        makeRecipient({ id: 'r3', name: 'Cleo', status: 'DECLINED' }),
        makeRecipient({ id: 'r4', name: 'Dan', status: 'PENDING' }),
      ],
    })
    expect(screen.getAllByText('Copy signing link')).toHaveLength(2)
  })

  it.each(['DRAFT', 'AWAITING_INPUT', 'PREPARING_SIGNATURE', 'COMPLETED', 'CANCELLED'])(
    'offers no link while the document is %s',
    (documentStatus) => {
      renderTable({ documentStatus })
      expect(screen.queryByText('Copy signing link')).toBeNull()
    },
  )

  it('offers no link to someone who cannot send documents', () => {
    renderTable({ canShareLinks: false })
    expect(screen.queryByText('Copy signing link')).toBeNull()
  })

  it("copies the signer's link", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    // happy-dom's navigator.clipboard is a getter, so redefine it.
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    vi.mocked(signingApi.getSigningURL).mockResolvedValue({
      signingUrl: 'https://sign.example.test/sign/tok-1',
    })

    renderTable()
    fireEvent.click(screen.getByText('Copy signing link'))

    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith('https://sign.example.test/sign/tok-1'),
    )
    expect(signingApi.getSigningURL).toHaveBeenCalledWith('doc-1', 'r1')
  })

  it('says when the link is not ready yet (409)', async () => {
    vi.mocked(signingApi.getSigningURL).mockRejectedValue(
      new AxiosError('conflict', 'ERR_BAD_REQUEST', undefined, undefined, {
        status: 409,
        statusText: 'Conflict',
        data: {},
        headers: {},
        config: { headers: new AxiosHeaders() },
      }),
    )

    renderTable()
    fireEvent.click(screen.getByText('Copy signing link'))

    await waitFor(() => expect(toast).toHaveBeenCalled())
    expect(toast.mock.calls[0][0].description).toContain("isn't ready yet")
  })

  it('shows formatted signedAt date when available', () => {
    renderTable({
      recipients: [makeRecipient({ signedAt: '2026-02-15T14:30:00Z' })],
    })
    // The formatted date should be present (locale-dependent, just check it's not "-")
    const cells = screen.queryAllByText('-')
    expect(cells).toHaveLength(0) // no dash since signedAt is provided
  })

  it('shows dash when signedAt is not available', () => {
    renderTable({ recipients: [makeRecipient({ signedAt: undefined })] })
    expect(screen.getByText('-')).toBeDefined()
  })
})
