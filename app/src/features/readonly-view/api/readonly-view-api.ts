import axios from 'axios'
import type { ReadOnlyViewResponse } from '../types'

const BASE_PATH = (import.meta.env.VITE_BASE_PATH || '').replace(/\/$/, '')

const readonlyPublicApi = axios.create({
  baseURL: BASE_PATH,
  headers: { 'Content-Type': 'application/json' },
  timeout: 60_000,
})

export async function getReadOnlyView(
  token: string,
): Promise<ReadOnlyViewResponse> {
  const { data } = await readonlyPublicApi.get<ReadOnlyViewResponse>(
    `/public/view/${token}`,
  )
  return data
}

export function getReadOnlyViewPdfUrl(token: string): string {
  return `${BASE_PATH}/public/view/${token}/pdf`
}
