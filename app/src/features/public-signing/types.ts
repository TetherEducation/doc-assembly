/**
 * Types for the public signing page.
 *
 * These mirror the backend DTO returned by GET /public/sign/:token
 * and the request/response bodies for the signing flow endpoints.
 */

export interface InteractiveFieldOption {
  id: string
  label: string
}

export interface InteractiveFieldDefinition {
  id: string
  fieldType: 'checkbox' | 'radio' | 'text'
  roleId: string
  label: string
  required: boolean
  options: InteractiveFieldOption[]
  placeholder: string
  maxLength: number
}

export interface SignerInfo {
  name: string
  email: string
  roleId: string
  roleName: string
}

export interface PreSigningFormData {
  documentTitle: string
  documentStatus: string
  recipientName: string
  recipientEmail: string
  roleId: string
  /** ProseMirror JSON document content with injectors already resolved */
  content: Record<string, unknown>
  /** Interactive field definitions that belong to THIS signer */
  fields: InteractiveFieldDefinition[]
}

/**
 * PublicSigningResponse mirrors the backend PublicSigningResponse.
 * The `step` field determines which UI to render.
 */
export interface PublicSigningResponse {
  step:
    | 'preview'
    | 'signing'
    | 'waiting'
    | 'completed'
    | 'declined'
    | 'processing'
    | 'document_updated'
    | 'unavailable'
  form?: PreSigningFormData
  pdfUrl?: string
  embeddedSigningUrl?: string
  documentTitle: string
  recipientName: string
  documentStatus?: string
  processingReason?: 'preparing_document' | 'recovering_provider_submission'
  retryAfterSeconds?: number
  supportCode?: string
  hasCurrentUserSigned: boolean
  canSign: boolean
  canDownload: boolean
  downloadUrl?: string
  waitingForPrevious?: boolean
  signingPosition?: number
  totalSigners?: number
  fallbackUrl?: string
}

export interface FieldResponsePayload {
  fieldId: string
  fieldType: string
  response: {
    selectedOptionIds?: string[]
    text?: string
  }
}

/**
 * DocumentAccessInfo mirrors the backend PublicDocumentInfoResponse.
 * Used by the public document access page (email-verification gate).
 */
export interface DocumentAccessInfo {
  documentId: string
  documentTitle: string
  status: 'active' | 'completed' | 'expired'
}

/**
 * Local form state: tracks the user's responses to each field.
 */
export type FieldResponses = Record<
  string,
  {
    fieldType: string
    response: { selectedOptionIds?: string[]; text?: string }
  }
>
