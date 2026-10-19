import { createFileRoute } from '@tanstack/react-router'

import { TrackingPage } from '../../../../pages/workspace/settings/tracking.tsx'

export const Route = createFileRoute('/workspaces/$slug/settings/tracking')({
  component: TrackingPage,
})
