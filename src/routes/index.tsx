import { createFileRoute, redirect } from '@tanstack/react-router'

import { requireWorkspaces } from '../guards.ts'
import { Route as overviewRoute } from './workspaces/$workspaceId/index.tsx'
import { Route as newWorkspaceRoute } from './workspaces/new.tsx'

export const Route = createFileRoute('/')({
  beforeLoad: async ({ context }) => {
    const [first] = await requireWorkspaces(context.queryClient)
    if (!first) {
      throw redirect({ to: newWorkspaceRoute.to })
    }
    throw redirect({ to: overviewRoute.to, params: { workspaceId: first.id } })
  },
})
