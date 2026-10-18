import { createFileRoute } from '@tanstack/react-router'

import { BroadcastCreatePage } from '../../../../pages/broadcasts/resource.tsx'

export const Route = createFileRoute('/workspaces/$slug/broadcasts/new')({
  component: BroadcastCreatePage,
})
