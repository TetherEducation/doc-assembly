import axios from 'axios'
import type {
  PublicSigningResponse,
  FieldResponsePayload,
  DocumentAccessInfo,
} from '../types'
import {
  appendPublicLanguage,
  type PublicSigningLanguage,
} from '../public-signing-language'

interface PublicLanguageOptions {
  language?: PublicSigningLanguage
}

/**
 * Standalone Axios instance for public endpoints.
 * No auth interceptor -- these endpoints require no JWT.
 * Base path does NOT include /api/v1 since public signing routes are at /public/sign/:token.
 */
const BASE_PATH = (import.meta.env.VITE_BASE_PATH || '').replace(/\/$/, '')

const publicApi = axios.create({
  baseURL: BASE_PATH,
  headers: { 'Content-Type': 'application/json' },
  timeout: 60_000,
})

export async function getPublicSigningPage(
  token: string,
  options: PublicLanguageOptions = {},
): Promise<PublicSigningResponse> {
  const { data } = await publicApi.get<PublicSigningResponse>(
    appendPublicLanguage(`/public/sign/${token}`, options.language),
  )
  return data
}

export async function requestDocumentAccessFromToken(
  token: string,
  email: string,
  options: PublicLanguageOptions = {},
): Promise<{ message: string }> {
  const { data } = await publicApi.post<{ message: string }>(
    appendPublicLanguage(`/public/sign/${token}/request-access`, options.language),
    { email },
  )
  return data
}

export async function submitPreSigningForm(
  token: string,
  responses: FieldResponsePayload[],
  options: PublicLanguageOptions = {},
): Promise<PublicSigningResponse> {
  const { data } = await publicApi.post<PublicSigningResponse>(
    appendPublicLanguage(`/public/sign/${token}`, options.language),
    { responses },
  )
  return data
}

export async function proceedToSigning(
  token: string,
  options: PublicLanguageOptions = {},
): Promise<PublicSigningResponse> {
  const { data } = await publicApi.post<PublicSigningResponse>(
    appendPublicLanguage(`/public/sign/${token}/proceed`, options.language),
  )
  return data
}

export async function completeEmbeddedSigning(
  token: string,
  options: PublicLanguageOptions = {},
): Promise<void> {
  await publicApi.post(
    appendPublicLanguage(`/public/sign/${token}/complete`, options.language),
  )
}

export async function fetchPreviewPDF(
  token: string,
  options: PublicLanguageOptions = {},
): Promise<ArrayBuffer> {
  const { data } = await publicApi.get(
    appendPublicLanguage(`/public/sign/${token}/pdf`, options.language),
    {
      responseType: 'arraybuffer',
    },
  )
  return data
}

export async function refreshEmbeddedUrl(
  token: string,
  options: PublicLanguageOptions = {},
): Promise<PublicSigningResponse> {
  const { data } = await publicApi.get<PublicSigningResponse>(
    appendPublicLanguage(`/public/sign/${token}/refresh`, options.language),
  )
  return data
}

export async function getDocumentAccessInfo(
  documentId: string,
  options: PublicLanguageOptions = {},
): Promise<DocumentAccessInfo> {
  const { data } = await publicApi.get<DocumentAccessInfo>(
    appendPublicLanguage(`/public/doc/${documentId}`, options.language),
  )
  return data
}

export async function requestDocumentAccess(
  documentId: string,
  email: string,
  options: PublicLanguageOptions = {},
): Promise<{ message: string }> {
  const { data } = await publicApi.post<{ message: string }>(
    appendPublicLanguage(`/public/doc/${documentId}/request-access`, options.language),
    { email },
  )
  return data
}
