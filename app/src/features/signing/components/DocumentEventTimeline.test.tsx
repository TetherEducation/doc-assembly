import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import type { DocumentEvent } from '../types'
import { DocumentEventTimeline } from './DocumentEventTimeline'
import { eventRecipient } from './eventRecipient'

let mockEvents: DocumentEvent[] = []

vi.mock('../hooks/useDocumentEvents', () => ({
  useDocumentEvents: () => ({ data: mockEvents, isLoading: false }),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (_key: string, fallback: string) => fallback }),
}))

function event(partial: Partial<DocumentEvent>): DocumentEvent {
  return {
    id: crypto.randomUUID(),
    documentId: 'doc-1',
    eventType: 'DOCUMENT_CREATED',
    actorType: 'SYSTEM',
    createdAt: '2026-11-11T10:00:00Z',
    ...partial,
  }
}

describe('eventRecipient', () => {
  it('shows name and email', () => {
    expect(
      eventRecipient({ recipientName: 'Ana Pérez', recipientEmail: 'ana@x.cl' }),
    ).toBe('Ana Pérez (ana@x.cl)')
  })
  it('falls back to whichever is present', () => {
    expect(eventRecipient({ recipientEmail: 'ana@x.cl' })).toBe('ana@x.cl')
    expect(eventRecipient({ recipientName: 'Ana' })).toBe('Ana')
  })
  it('does not repeat an email used as the name', () => {
    expect(
      eventRecipient({ recipientName: 'ana@x.cl', recipientEmail: 'ana@x.cl' }),
    ).toBe('ana@x.cl')
  })
  it('returns null without a recipient', () => {
    expect(eventRecipient(undefined)).toBeNull()
    expect(eventRecipient({ attemptId: 'a1' })).toBeNull()
  })
})

describe('DocumentEventTimeline', () => {
  it('labels DELIVERED as viewed and names the signer on recipient events', () => {
    mockEvents = [
      event({ eventType: 'DOCUMENT_CREATED', createdAt: '2026-11-11T10:00:00Z' }),
      event({
        eventType: 'RECIPIENT_DELIVERED',
        createdAt: '2026-11-11T10:05:00Z',
        metadata: { recipientName: 'Signer One', recipientEmail: 's1@example.com' },
      }),
      event({
        eventType: 'RECIPIENT_SIGNED',
        createdAt: '2026-11-11T10:06:00Z',
        metadata: { recipientName: 'Signer Two', recipientEmail: 's2@example.com' },
      }),
    ]
    render(<DocumentEventTimeline documentId="doc-1" />)
    expect(screen.getByText('Recipient viewed')).toBeDefined()
    expect(screen.queryByText(/Delivered to recipient/)).toBeNull()
    expect(screen.getByText(/Signer One \(s1@example\.com\)/)).toBeDefined()
    expect(screen.getByText(/Signer Two \(s2@example\.com\)/)).toBeDefined()
  })

  it('does not add a signer to document-level events', () => {
    mockEvents = [
      event({
        eventType: 'DOCUMENT_COMPLETED',
        metadata: { attemptId: 'a1', recipientEmail: 'stray@example.com' },
      }),
    ]
    render(<DocumentEventTimeline documentId="doc-1" />)
    expect(screen.getByText('Document completed')).toBeDefined()
    expect(screen.queryByText(/stray@example\.com/)).toBeNull()
  })
})
