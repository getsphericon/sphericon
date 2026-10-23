import { createFileRoute } from '@tanstack/react-router'

import { requireWorkspaces } from '../../guards.ts'

export const Route = createFileRoute('/account')({
  // Workspace-independent settings: authenticates, but renders its own shell.
  beforeLoad: async ({ context, location }) => {
    await requireWorkspaces(context.queryClient, location.href)
  },
})
