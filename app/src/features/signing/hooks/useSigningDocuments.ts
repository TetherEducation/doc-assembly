import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { signingApi } from '../api/signing-api'
import type { CreateDocumentRequest, DocumentListFilters } from '../types'

// Query keys factory
export const signingKeys = {
  all: ['signing-documents'] as const,
  lists: () => [...signingKeys.all, 'list'] as const,
  list: (filters?: DocumentListFilters) =>
    [...signingKeys.lists(), filters] as const,
  details: () => [...signingKeys.all, 'detail'] as const,
  detail: (id: string) => [...signingKeys.details(), id] as const,
  statistics: () => [...signingKeys.all, 'statistics'] as const,
  events: (docId: string) => [...signingKeys.all, 'events', docId] as const,
}

export function useSigningDocuments(filters?: DocumentListFilters) {
  return useQuery({
    queryKey: signingKeys.list(filters),
    queryFn: () => signingApi.list(filters),
  })
}

export function useSigningDocument(id: string) {
  return useQuery({
    queryKey: signingKeys.detail(id),
    queryFn: () => signingApi.getById(id),
    enabled: !!id,
  })
}

export function useCreateDocument() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (data: CreateDocumentRequest) => signingApi.create(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: signingKeys.lists() })
    },
  })
}

export function useCancelDocument() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (id: string) => signingApi.cancel(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: signingKeys.lists() })
      queryClient.invalidateQueries({ queryKey: signingKeys.detail(id) })
    },
  })
}

export function useDeprecateDocument() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ id, reason }: { id: string; reason?: string }) =>
      signingApi.deprecate(id, { reason }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: signingKeys.lists() })
      queryClient.invalidateQueries({ queryKey: signingKeys.detail(variables.id) })
    },
  })
}

export function useRefreshDocument() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (id: string) => signingApi.refresh(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: signingKeys.detail(id) })
    },
  })
}

export function useInvalidateTokens() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (docId: string) => signingApi.invalidateTokens(docId),
    onSuccess: (_data, docId) => {
      queryClient.invalidateQueries({ queryKey: signingKeys.detail(docId) })
    },
  })
}

export function useSigningURL(docId: string, recipientId: string) {
  return useQuery({
    queryKey: [...signingKeys.detail(docId), 'signing-url', recipientId] as const,
    queryFn: () => signingApi.getSigningURL(docId, recipientId),
    enabled: !!docId && !!recipientId,
  })
}
