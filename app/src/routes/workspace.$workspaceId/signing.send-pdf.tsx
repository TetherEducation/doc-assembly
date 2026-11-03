import { createFileRoute, redirect } from '@tanstack/react-router'
import { SendPdfPage } from '@/features/pdf-send/components/SendPdfPage'
import { getAuthConfig } from '@/lib/auth-config'
import { redirectSystemWorkspaceProductSurface } from '@/lib/system-workspace-guard'

export const Route = createFileRoute('/workspace/$workspaceId/signing/send-pdf')({
  beforeLoad: async ({ params }) => {
    redirectSystemWorkspaceProductSurface(params)
    // Only deployments that turn on signing.pdf_send offer this page; a
    // direct link elsewhere goes back to the signing list.
    const config = await getAuthConfig()
    if (!config.features?.pdfSend) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- TanStack Router type limitation
      throw redirect({ to: '/workspace/$workspaceId/signing', params: params as any })
    }
  },
  component: SendPdfPage,
})
