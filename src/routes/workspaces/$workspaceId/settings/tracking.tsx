import { createFileRoute } from '@tanstack/react-router'

import { TrackingPage } from '../../../../pages/workspace/settings/tracking.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/settings/tracking')({
  component: TrackingPage,
})
