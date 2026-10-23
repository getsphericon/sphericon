import { createLazyFileRoute } from '@tanstack/react-router'

import { BroadcastEditPage } from '../../../../../pages/broadcasts/resource.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/broadcasts/$broadcastId/edit')({
  component: BroadcastEditPage,
})
