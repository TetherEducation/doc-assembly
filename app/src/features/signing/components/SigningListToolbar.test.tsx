import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { SigningListToolbar } from './SigningListToolbar'

// Mock react-i18next
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback: string) => fallback,
  }),
}))

describe('SigningListToolbar', () => {
  const defaultProps = {
    searchQuery: '',
    onSearchChange: vi.fn(),
    documentTypeOptions: [] as { id: string; name: string }[],
    selectedDocumentTypeIds: [] as string[],
    onDocumentTypesChange: vi.fn(),
    selectedStatuses: [] as string[],
    onStatusesChange: vi.fn(),
  }

  it('renders search input with placeholder', () => {
    render(<SigningListToolbar {...defaultProps} />)
    expect(
      screen.getByPlaceholderText(
        'Search by document title or signer email...',
      ),
    ).toBeDefined()
  })

  it('displays current search query value', () => {
    render(<SigningListToolbar {...defaultProps} searchQuery="test query" />)
    const input = screen.getByPlaceholderText(
      'Search by document title or signer email...',
    ) as HTMLInputElement
    expect(input.value).toBe('test query')
  })

  it('fires onSearchChange when typing in search input', async () => {
    const onSearchChange = vi.fn()
    render(
      <SigningListToolbar {...defaultProps} onSearchChange={onSearchChange} />,
    )

    const input = screen.getByPlaceholderText(
      'Search by document title or signer email...',
    )
    await userEvent.type(input, 'hello')

    // Called once per character; each call receives the single char typed
    // because the component is controlled and searchQuery prop doesn't change
    expect(onSearchChange).toHaveBeenCalledTimes(5)
    expect(onSearchChange).toHaveBeenNthCalledWith(1, 'h')
    expect(onSearchChange).toHaveBeenNthCalledWith(5, 'o')
  })

  it('shows "Any" when no statuses are selected', () => {
    render(<SigningListToolbar {...defaultProps} />)
    expect(screen.getByText(/Status.*Any/)).toBeDefined()
  })

  it('shows count when statuses are selected', () => {
    render(
      <SigningListToolbar
        {...defaultProps}
        selectedStatuses={['PENDING', 'COMPLETED']}
      />,
    )
    expect(screen.getByText(/Status.*2/)).toBeDefined()
  })

  it('opens dropdown and shows status options when clicking status button', async () => {
    render(<SigningListToolbar {...defaultProps} />)

    const statusButton = screen.getByText(/Status.*Any/)
    await userEvent.click(statusButton)

    expect(screen.getByText('Draft')).toBeDefined()
    expect(screen.getByText('Ready to Sign')).toBeDefined()
    expect(screen.getByText('Signing')).toBeDefined()
    expect(screen.getByText('Completed')).toBeDefined()
    expect(screen.getByText('Declined')).toBeDefined()
    expect(screen.getByText('Cancelled')).toBeDefined()
    expect(screen.getByText('Invalidated')).toBeDefined()
    expect(screen.getByText('Error')).toBeDefined()
  })

  it('calls onStatusesChange with added status when clicking a status option', async () => {
    const onStatusesChange = vi.fn()
    render(
      <SigningListToolbar
        {...defaultProps}
        onStatusesChange={onStatusesChange}
      />,
    )

    // Open dropdown
    await userEvent.click(screen.getByText(/Status.*Any/))
    // Click "Completed"
    await userEvent.click(screen.getByText('Completed'))

    expect(onStatusesChange).toHaveBeenCalledWith(['COMPLETED'])
  })

  it('calls onStatusesChange removing status when clicking an already-selected status', async () => {
    const onStatusesChange = vi.fn()
    render(
      <SigningListToolbar
        {...defaultProps}
        selectedStatuses={['PENDING', 'COMPLETED']}
        onStatusesChange={onStatusesChange}
      />,
    )

    // Open dropdown
    await userEvent.click(screen.getByText(/Status.*2/))
    // Click "COMPLETED" to deselect
    await userEvent.click(screen.getByText('Completed'))

    expect(onStatusesChange).toHaveBeenCalledWith(['PENDING'])
  })

  it('clears all statuses when clicking "Clear all"', async () => {
    const onStatusesChange = vi.fn()
    render(
      <SigningListToolbar
        {...defaultProps}
        selectedStatuses={['PENDING', 'COMPLETED']}
        onStatusesChange={onStatusesChange}
      />,
    )

    // Open dropdown
    await userEvent.click(screen.getByText(/Status.*2/))
    // Click "Clear all"
    await userEvent.click(screen.getByText('Clear all'))

    expect(onStatusesChange).toHaveBeenCalledWith([])
  })

  it('shows All types when no document types are selected', () => {
    render(<SigningListToolbar {...defaultProps} />)
    expect(screen.getByText(/Template type.*All types/)).toBeDefined()
  })

  it('opens template type dropdown and toggles selection', async () => {
    const onDocumentTypesChange = vi.fn()
    render(
      <SigningListToolbar
        {...defaultProps}
        documentTypeOptions={[
          { id: 'dt-1', name: 'NDA' },
          { id: 'dt-2', name: 'Employment Agreement' },
        ]}
        onDocumentTypesChange={onDocumentTypesChange}
      />,
    )

    await userEvent.click(screen.getByText(/Template type.*All types/))
    await userEvent.click(screen.getByText('NDA'))

    expect(onDocumentTypesChange).toHaveBeenCalledWith(['dt-1'])
  })
})
