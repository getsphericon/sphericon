import { createLazyFileRoute } from '@tanstack/react-router'

import { BroadcastsListPage } from '../../../../pages/broadcasts/list.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/broadcasts/')({
  component: BroadcastsListPage,
})
