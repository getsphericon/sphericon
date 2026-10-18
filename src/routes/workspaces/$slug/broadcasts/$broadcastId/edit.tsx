import { createFileRoute } from '@tanstack/react-router'

import { BroadcastEditPage } from '../../../../../pages/broadcasts/resource.tsx'

export const Route = createFileRoute('/workspaces/$slug/broadcasts/$broadcastId/edit')({
  component: BroadcastEditPage,
})
