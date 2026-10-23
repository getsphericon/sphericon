import { createLazyFileRoute } from '@tanstack/react-router'

import { WorkspaceLayout } from '../../../layouts/WorkspaceLayout.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId')({
  component: WorkspaceLayout,
})
