import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'
import { TemplateDetailPage } from '@/features/templates/components/TemplateDetailPage'
import { redirectSystemWorkspaceProductSurface } from '@/lib/system-workspace-guard'

const templateDetailSearchSchema = z.object({
  fromFolderId: z.string().optional(),
})

export const Route = createFileRoute('/workspace/$workspaceId/templates/$templateId')({
  beforeLoad: ({ params }) => redirectSystemWorkspaceProductSurface(params),
  component: TemplateDetailPage,
  validateSearch: templateDetailSearchSchema,
})
