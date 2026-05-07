import apiClient from '@/lib/api-client'
import type {
  SigningDocumentListItem,
  SigningDocumentDetail,
  CreateDocumentRequest,
  DocumentStatistics,
  DocumentEvent,
  SigningURLResponse,
  DocumentListFilters,
  DeprecateDocumentRequest,
  DeprecateDocumentResponse,
} from '../types'

const BASE_PATH = '/documents'

export const signingApi = {
  list: (filters?: DocumentListFilters) =>
    apiClient
      .get<SigningDocumentListItem[]>(BASE_PATH, { params: filters })
      .then((r) => r.data),

  getById: (id: string) =>
    apiClient
      .get<SigningDocumentDetail>(`${BASE_PATH}/${id}`)
      .then((r) => r.data),

  create: (req: CreateDocumentRequest) =>
    apiClient
      .post<SigningDocumentDetail>(BASE_PATH, req)
      .then((r) => r.data),

  cancel: (id: string) =>
    apiClient
      .post<void>(`${BASE_PATH}/${id}/cancel`)
      .then((r) => r.data),

  deprecate: (id: string, req: DeprecateDocumentRequest = {}) =>
    apiClient
      .post<DeprecateDocumentResponse>(`${BASE_PATH}/${id}/deprecate`, req)
      .then((r) => r.data),

  refresh: (id: string) =>
    apiClient
      .post<SigningDocumentDetail>(`${BASE_PATH}/${id}/refresh`)
      .then((r) => r.data),

  getSigningURL: (docId: string, recipientId: string) =>
    apiClient
      .get<SigningURLResponse>(
        `${BASE_PATH}/${docId}/recipients/${recipientId}/signing-url`
      )
      .then((r) => r.data),

  getStatistics: () =>
    apiClient
      .get<DocumentStatistics>(`${BASE_PATH}/statistics`)
      .then((r) => r.data),

  getEvents: (docId: string) =>
    apiClient
      .get<DocumentEvent[]>(`${BASE_PATH}/${docId}/events`)
      .then((r) => r.data),

  downloadPDF: (docId: string) =>
    apiClient
      .get<Blob>(`${BASE_PATH}/${docId}/pdf`, { responseType: 'blob' })
      .then((r) => r.data),

  invalidateTokens: (docId: string) =>
    apiClient
      .post<void>(`${BASE_PATH}/${docId}/invalidate-tokens`)
      .then((r) => r.data),
}
