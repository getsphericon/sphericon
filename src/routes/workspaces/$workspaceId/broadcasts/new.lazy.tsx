import { createLazyFileRoute } from '@tanstack/react-router'

import { BroadcastCreatePage } from '../../../../pages/broadcasts/resource.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/broadcasts/new')({
  component: BroadcastCreatePage,
})
