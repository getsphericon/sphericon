import { createLazyFileRoute } from '@tanstack/react-router'

import { EmailMetricsPage } from '../../../../pages/workspace/analytics/emailMetrics.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/analytics/email-metrics')({
  component: EmailMetricsPage,
})
