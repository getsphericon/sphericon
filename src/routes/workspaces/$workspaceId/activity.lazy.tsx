import { createLazyFileRoute } from '@tanstack/react-router'

import { ActivityPage } from '../../../pages/workspace/activity.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/activity')({
  component: ActivityPage,
})
