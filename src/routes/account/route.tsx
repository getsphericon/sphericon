import { createFileRoute } from '@tanstack/react-router'

import { requireWorkspaces } from '../../guards.ts'
import { AccountLayout } from '../../layouts/AccountLayout.tsx'

export const Route = createFileRoute('/account')({
  // Workspace-independent settings: authenticates, but renders its own shell.
  beforeLoad: async ({ context }) => {
    await requireWorkspaces(context.queryClient)
  },
  component: AccountLayout,
})
