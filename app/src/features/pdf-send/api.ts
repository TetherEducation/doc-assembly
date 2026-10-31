import { apiClient } from '@/lib/api-client'
import type { SigningDocumentDetail } from '@/features/signing/types'

import type { SendPdfPayload } from './model'

// The engine stores the PDF, places the boxes, publishes a one-off template
// and creates the document in one request, so allow more than the client's
// default 30 s.
const SEND_TIMEOUT_MS = 120_000

export const pdfSendApi = {
  async send(file: File, payload: SendPdfPayload): Promise<SigningDocumentDetail> {
    const form = new FormData()
    form.append('file', file)
    form.append('payload', JSON.stringify(payload))
    const response = await apiClient.post<SigningDocumentDetail>('/documents/from-pdf', form, {
      // Let the browser set multipart/form-data with its boundary.
      headers: { 'Content-Type': undefined },
      timeout: SEND_TIMEOUT_MS,
    })
    return response.data
  },
}
