import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { CreateDocumentWizard } from './CreateDocumentWizard'

// Mock react-i18next
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback: string) => fallback,
  }),
}))

const { mutateAsync, templates } = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
  // Two templates, one version each, with no fill-in key in common
  templates: {
    nda: {
      title: 'NDA',
      versions: [
        {
          id: 'nda-v1',
          signerRoles: [],
          injectables: [{ definition: { key: 'counterparty_name' }, isRequired: false }],
        },
      ],
    },
    offer: {
      title: 'Offer',
      versions: [
        {
          id: 'offer-v1',
          signerRoles: [],
          injectables: [{ definition: { key: 'job_title' }, isRequired: false }],
        },
      ],
    },
  } as Record<string, unknown>,
}))

// Mock useCreateDocument
vi.mock('../hooks/useSigningDocuments', () => ({
  useCreateDocument: () => ({
    mutateAsync,
    isPending: false,
  }),
}))

// Mock useTemplateWithVersions
vi.mock('@/features/templates/hooks/useTemplateDetail', () => ({
  useTemplateWithVersions: (templateId: string) => ({
    data: templates[templateId] ?? null,
  }),
}))

// Mock child wizard step components to simplify tests
vi.mock('./WizardStepVersion', () => ({
  WizardStepVersion: ({
    onTemplateChange,
    onVersionChange,
  }: {
    onTemplateChange: (id: string) => void
    onVersionChange: (id: string) => void
  }) => (
    <div data-testid="step-version">
      Version Step
      <button type="button" onClick={() => { onTemplateChange('nda'); onVersionChange('nda-v1') }}>
        pick nda
      </button>
      <button type="button" onClick={() => { onTemplateChange('offer'); onVersionChange('offer-v1') }}>
        pick offer
      </button>
    </div>
  ),
}))

vi.mock('./WizardStepValues', () => ({
  WizardStepValues: ({
    values,
    onValuesChange,
  }: {
    values: Record<string, unknown>
    onValuesChange: (v: Record<string, unknown>) => void
  }) => (
    <div data-testid="step-values">
      Values Step
      <button type="button" onClick={() => onValuesChange({ ...values, counterparty_name: 'Acme' })}>
        fill nda
      </button>
      <button type="button" onClick={() => onValuesChange({ ...values, job_title: 'Analyst' })}>
        fill offer
      </button>
    </div>
  ),
}))

vi.mock('./WizardStepRecipients', () => ({
  WizardStepRecipients: () => (
    <div data-testid="step-recipients">Recipients Step</div>
  ),
}))

vi.mock('./WizardStepReview', () => ({
  WizardStepReview: ({ values }: { values: Record<string, unknown> }) => (
    <div data-testid="step-review">{JSON.stringify(values)}</div>
  ),
}))

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return function Wrapper({ children }: { children: React.ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    )
  }
}

describe('CreateDocumentWizard', () => {
  it('does not render when open is false', () => {
    render(
      <CreateDocumentWizard
        open={false}
        onOpenChange={vi.fn()}
      />,
      { wrapper: createWrapper() },
    )
    expect(screen.queryByText('Create Signing Document')).toBeNull()
  })

  it('renders dialog title and first step when open', () => {
    render(
      <CreateDocumentWizard
        open={true}
        onOpenChange={vi.fn()}
      />,
      { wrapper: createWrapper() },
    )
    expect(screen.getByText('Create Signing Document')).toBeDefined()
    expect(screen.getByTestId('step-version')).toBeDefined()
  })

  it('shows step indicators', () => {
    render(
      <CreateDocumentWizard
        open={true}
        onOpenChange={vi.fn()}
      />,
      { wrapper: createWrapper() },
    )
    // "Template & Version" appears in both the step indicator and description
    expect(screen.getAllByText(/Template & Version/).length).toBeGreaterThanOrEqual(1)
    // These step labels appear in the indicator bar
    expect(screen.getAllByText(/Values/).length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText(/Recipients/).length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText(/Review/).length).toBeGreaterThanOrEqual(1)
  })

  it('shows Document Title input on first step', () => {
    render(
      <CreateDocumentWizard
        open={true}
        onOpenChange={vi.fn()}
      />,
      { wrapper: createWrapper() },
    )
    expect(
      screen.getByPlaceholderText('Enter document title...'),
    ).toBeDefined()
  })

  it('Next button is disabled when title is empty (first step)', () => {
    render(
      <CreateDocumentWizard
        open={true}
        onOpenChange={vi.fn()}
      />,
      { wrapper: createWrapper() },
    )
    const nextButton = screen.getByText('Next')
    expect((nextButton as HTMLButtonElement).disabled).toBe(true)
  })

  it('does not show Back button on first step', () => {
    render(
      <CreateDocumentWizard
        open={true}
        onOpenChange={vi.fn()}
      />,
      { wrapper: createWrapper() },
    )
    expect(screen.queryByText('Back')).toBeNull()
  })

  it('allows typing a document title', async () => {
    render(
      <CreateDocumentWizard
        open={true}
        onOpenChange={vi.fn()}
      />,
      { wrapper: createWrapper() },
    )
    const titleInput = screen.getByPlaceholderText('Enter document title...')
    await userEvent.type(titleInput, 'My Contract')
    expect((titleInput as HTMLInputElement).value).toBe('My Contract')
  })

  it('reopens on the first step with an empty form', async () => {
    const user = userEvent.setup()
    const { rerender } = render(
      <CreateDocumentWizard open={true} onOpenChange={vi.fn()} />,
      { wrapper: createWrapper() },
    )
    await user.type(screen.getByPlaceholderText('Enter document title...'), 'Old draft')
    await user.click(screen.getByText('pick nda'))
    await user.click(screen.getByText('Next'))
    expect(screen.getByTestId('step-values')).toBeDefined()

    rerender(<CreateDocumentWizard open={false} onOpenChange={vi.fn()} />)
    rerender(<CreateDocumentWizard open={true} onOpenChange={vi.fn()} />)

    expect(screen.getByTestId('step-version')).toBeDefined()
    expect(
      (screen.getByPlaceholderText('Enter document title...') as HTMLInputElement).value,
    ).toBe('')
  })

  it("sends only the selected version's values after a template switch", async () => {
    const user = userEvent.setup()
    mutateAsync.mockResolvedValue({ id: 'doc-1', status: 'PENDING' })
    render(
      <CreateDocumentWizard open={true} onOpenChange={vi.fn()} />,
      { wrapper: createWrapper() },
    )
    await user.type(screen.getByPlaceholderText('Enter document title...'), 'Offer')
    await user.click(screen.getByText('pick nda'))
    await user.click(screen.getByText('Next'))
    await user.click(screen.getByText('fill nda'))
    await user.click(screen.getByText('Back'))
    await user.click(screen.getByText('pick offer'))
    await user.click(screen.getByText('Next'))
    await user.click(screen.getByText('fill offer'))
    await user.click(screen.getByText('Next'))
    await user.click(screen.getByText('Next'))

    expect(screen.getByTestId('step-review').textContent).toBe('{"job_title":"Analyst"}')
    await user.click(screen.getByText('Send for Signing'))
    expect(mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({ injectedValues: { job_title: 'Analyst' } }),
    )
  })
})
