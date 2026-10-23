import { createFileRoute, redirect } from '@tanstack/react-router'

import { requireWorkspaces } from '../../../guards.ts'
import { Route as newWorkspaceRoute } from '../new.tsx'
import { Route as overviewRoute } from './index.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId')({
  // Authenticates and verifies the Workspace id is one the user belongs to.
  beforeLoad: async ({ context, params, location }) => {
    const workspaces = await requireWorkspaces(context.queryClient, location.href)
    if (!workspaces.some((w) => w.id === params.workspaceId)) {
      const [fallback] = workspaces
      if (!fallback) {
        throw redirect({ to: newWorkspaceRoute.to })
      }
      throw redirect({ to: overviewRoute.to, params: { workspaceId: fallback.id } })
    }
    return { workspaces }
  },
})
