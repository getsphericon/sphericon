import { createLazyFileRoute } from '@tanstack/react-router'

import { CreateWorkspacePage } from '../../pages/workspace/create.tsx'

export const Route = createLazyFileRoute('/workspaces/new')({
  component: CreateWorkspacePage,
})
