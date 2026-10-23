import { createLazyFileRoute } from '@tanstack/react-router'

import { RecentActivityPage } from '../../../../pages/workspace/analytics/recentActivity.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/analytics/recent-activity')({
  component: RecentActivityPage,
})
