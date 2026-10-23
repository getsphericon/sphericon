import { createLazyFileRoute } from '@tanstack/react-router'

import { AnalyticsPage } from '../../../pages/workspace/analytics.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/analytics')({
  component: AnalyticsPage,
})
