import { createFileRoute } from '@tanstack/react-router'
import { SigningListPage } from '@/features/signing/components/SigningListPage'
import { redirectSystemWorkspaceProductSurface } from '@/lib/system-workspace-guard'

export const Route = createFileRoute('/workspace/$workspaceId/signing/')({
  beforeLoad: ({ params }) => redirectSystemWorkspaceProductSurface(params),
  component: SigningListPage,
})
