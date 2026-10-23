import { createLazyFileRoute } from '@tanstack/react-router'

import { AnalyticsLayout } from '../../../../layouts/AnalyticsLayout.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/analytics')({
  component: AnalyticsLayout,
})
