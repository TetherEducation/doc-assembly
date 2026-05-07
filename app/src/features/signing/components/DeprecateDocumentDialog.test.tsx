import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { DeprecateDocumentDialog } from './DeprecateDocumentDialog'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback: string, opts?: Record<string, string>) => {
      if (opts?.title) return fallback.replace('{{title}}', opts.title)
      return fallback
    },
  }),
}))

vi.mock('@/components/ui/use-toast', () => ({
  useToast: () => ({ toast: vi.fn() }),
}))

const mockMutateAsync = vi.fn().mockResolvedValue({ id: 'doc-1', status: 'INVALIDATED' })

vi.mock('../hooks/useSigningDocuments', () => ({
  useDeprecateDocument: () => ({
    mutateAsync: mockMutateAsync,
    isPending: false,
  }),
}))

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }
}

describe('DeprecateDocumentDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders completed-document deprecation warning', () => {
    render(
      <DeprecateDocumentDialog
        open={true}
        onOpenChange={vi.fn()}
        documentId="doc-1"
        documentTitle="Signed Contract"
      />,
      { wrapper: createWrapper() },
    )

    expect(screen.getAllByText('Deprecate Document').length).toBeGreaterThanOrEqual(1)
    expect(
      screen.getByText('Are you sure you want to deprecate "Signed Contract"?'),
    ).toBeDefined()
    expect(
      screen.getByText('The signed document will be invalidated in doc-assembly and cleanup will be attempted in the signing provider.'),
    ).toBeDefined()
  })

  it('calls mutateAsync with id and reason', async () => {
    const onSuccess = vi.fn()
    render(
      <DeprecateDocumentDialog
        open={true}
        onOpenChange={vi.fn()}
        documentId="doc-123"
        documentTitle="Signed Contract"
        onSuccess={onSuccess}
      />,
      { wrapper: createWrapper() },
    )

    await userEvent.type(screen.getByLabelText('Reason'), 'replacement signed')
    const buttons = screen.getAllByText('Deprecate Document')
    await userEvent.click(buttons[buttons.length - 1])

    expect(mockMutateAsync).toHaveBeenCalledWith({
      id: 'doc-123',
      reason: 'replacement signed',
    })
    expect(onSuccess).toHaveBeenCalled()
  })
})
