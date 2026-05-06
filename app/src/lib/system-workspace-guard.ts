import { redirect } from '@tanstack/react-router'
import { useAppContextStore } from '@/stores/app-context-store'

type WorkspaceRouteParams = {
  workspaceId: string
}

export function redirectSystemWorkspaceProductSurface(params: WorkspaceRouteParams) {
  const { currentWorkspace } = useAppContextStore.getState()
  if (currentWorkspace?.type === 'SYSTEM') {
    throw redirect({ to: '/workspace/$workspaceId', params })
  }
}
