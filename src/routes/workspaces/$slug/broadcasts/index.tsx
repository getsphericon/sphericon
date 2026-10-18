import { createFileRoute } from '@tanstack/react-router'

import { BroadcastsListPage } from '../../../../pages/broadcasts/list.tsx'

export const Route = createFileRoute('/workspaces/$slug/broadcasts/')({
  component: BroadcastsListPage,
})
