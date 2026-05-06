import { createFileRoute } from '@tanstack/react-router'
import { TemplatesPage } from '@/features/templates'
import { redirectSystemWorkspaceProductSurface } from '@/lib/system-workspace-guard'

export const Route = createFileRoute('/workspace/$workspaceId/templates/')({
  beforeLoad: ({ params }) => redirectSystemWorkspaceProductSurface(params),
  component: TemplatesPage,
})
