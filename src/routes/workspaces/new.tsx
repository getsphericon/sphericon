import { createFileRoute } from '@tanstack/react-router'

import { requireWorkspaces } from '../../guards.ts'
import { CreateWorkspacePage } from '../../pages/workspace/create.tsx'

export const Route = createFileRoute('/workspaces/new')({
  // Authenticates; the list may be empty, which is exactly who lands here.
  beforeLoad: async ({ context, location }) => {
    await requireWorkspaces(context.queryClient, location.href)
  },
  component: CreateWorkspacePage,
})
