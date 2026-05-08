// Document statuses matching backend
export const SigningDocumentStatus = {
  DRAFT: 'DRAFT',
  AWAITING_INPUT: 'AWAITING_INPUT',
  PREPARING_SIGNATURE: 'PREPARING_SIGNATURE',
  READY_TO_SIGN: 'READY_TO_SIGN',
  SIGNING: 'SIGNING',
  COMPLETED: 'COMPLETED',
  DECLINED: 'DECLINED',
  CANCELLED: 'CANCELLED',
  INVALIDATED: 'INVALIDATED',
  ERROR: 'ERROR',
} as const
export type SigningDocumentStatus = (typeof SigningDocumentStatus)[keyof typeof SigningDocumentStatus]

// Recipient statuses
export const RecipientStatus = {
  PENDING: 'PENDING',
  SENT: 'SENT',
  DELIVERED: 'DELIVERED',
  SIGNED: 'SIGNED',
  DECLINED: 'DECLINED',
} as const
export type RecipientStatus = (typeof RecipientStatus)[keyof typeof RecipientStatus]

export interface SigningRecipient {
  id: string
  roleId: string
  roleName: string
  name: string
  email: string
  status: RecipientStatus
  signerOrder?: number
  signedAt?: string
  createdAt: string
  updatedAt: string
}

export interface SigningDocumentListItem {
  id: string
  workspaceId: string
  templateVersionId: string
  title: string
  clientExternalReferenceId?: string
  signerProvider?: string
  status: SigningDocumentStatus
  createdAt: string
  updatedAt: string
}

export interface FieldResponse {
  fieldId: string
  label: string
  fieldType: string
  value: unknown
}

export interface SigningDocumentDetail extends SigningDocumentListItem {
  recipients: SigningRecipient[]
  fieldResponses?: FieldResponse[]
}

export interface DeprecateDocumentRequest {
  reason?: string
}

export interface DeprecateDocumentResponse {
  id: string
  status: SigningDocumentStatus
  providerCleanup?: {
    action?: string
    status: string
    reason?: string
  }
}

export interface CreateDocumentRequest {
  templateVersionId: string
  title: string
  clientExternalReferenceId?: string
  injectedValues: Record<string, unknown>
  recipients: DocumentRecipientCommand[]
}

export interface DocumentRecipientCommand {
  roleId: string
  name: string
  email: string
}

export interface DocumentStatistics {
  total: number
  pending: number
  inProgress: number
  completed: number
  declined: number
  byStatus: Record<string, number>
}

export interface DocumentEvent {
  id: string
  documentId: string
  eventType: string
  actorType: string
  actorId?: string
  oldStatus?: string
  newStatus?: string
  recipientId?: string
  metadata?: Record<string, unknown>
  createdAt: string
}

export interface CreateViewLinkResponse {
  url: string
  token: string
  expiresAt: string
}

export interface SigningURLResponse {
  signingUrl: string
  expiresAt?: string
}

export interface DocumentListFilters {
  status?: string
  search?: string
  page?: number
  pageSize?: number
}
