import { createFileRoute } from '@tanstack/react-router'

import { AnalyticsPage } from '../../../pages/workspace/analytics.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/analytics')({
  component: AnalyticsPage,
})
