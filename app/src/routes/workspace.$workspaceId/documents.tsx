import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { DocumentsPage } from '@/features/documents'
import { redirectSystemWorkspaceProductSurface } from '@/lib/system-workspace-guard'

const documentsSearchSchema = z.object({
  folderId: z.string().optional(),
})

export const Route = createFileRoute('/workspace/$workspaceId/documents')({
  beforeLoad: ({ params }) => redirectSystemWorkspaceProductSurface(params),
  component: DocumentsPage,
  validateSearch: documentsSearchSchema,
})
