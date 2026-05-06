import { createFileRoute, Outlet } from '@tanstack/react-router'
import { redirectSystemWorkspaceProductSurface } from '@/lib/system-workspace-guard'

export const Route = createFileRoute('/workspace/$workspaceId/templates')({
  beforeLoad: ({ params }) => redirectSystemWorkspaceProductSurface(params),
  component: TemplatesLayout,
})

function TemplatesLayout() {
  return <Outlet />
}
