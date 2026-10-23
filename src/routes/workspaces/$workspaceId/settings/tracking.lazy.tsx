import { createLazyFileRoute } from '@tanstack/react-router'

import { TrackingPage } from '../../../../pages/workspace/settings/tracking.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/settings/tracking')({
  component: TrackingPage,
})
