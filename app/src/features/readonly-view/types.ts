export type ReadOnlyViewMode = 'content' | 'pdf' | 'unavailable'

export interface ReadOnlyViewResponse {
  mode: ReadOnlyViewMode
  documentId: string
  documentTitle: string
  documentStatus: string
  expiresAt: string
  content?: Record<string, unknown>
  pdfUrl?: string
  reason?: string
}
