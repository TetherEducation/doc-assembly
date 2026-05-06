import { createFileRoute } from '@tanstack/react-router'
import { SigningDetailPage } from '@/features/signing/components/SigningDetailPage'
import { redirectSystemWorkspaceProductSurface } from '@/lib/system-workspace-guard'

export const Route = createFileRoute(
  '/workspace/$workspaceId/signing/$documentId'
)({
  beforeLoad: ({ params }) => redirectSystemWorkspaceProductSurface(params),
  component: SigningDetailPage,
})
