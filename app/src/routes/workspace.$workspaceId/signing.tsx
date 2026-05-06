import { createFileRoute, Outlet } from '@tanstack/react-router'
import { redirectSystemWorkspaceProductSurface } from '@/lib/system-workspace-guard'

export const Route = createFileRoute('/workspace/$workspaceId/signing')({
  beforeLoad: ({ params }) => redirectSystemWorkspaceProductSurface(params),
  component: SigningLayout,
})

function SigningLayout() {
  return <Outlet />
}
