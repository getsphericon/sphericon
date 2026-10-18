import { createFileRoute } from '@tanstack/react-router'

import { ActivityPage } from '../../../pages/workspace/activity.tsx'

export const Route = createFileRoute('/workspaces/$slug/activity')({
  component: ActivityPage,
})
