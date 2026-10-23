import { createFileRoute, redirect } from '@tanstack/react-router'

import { Route as emailMetricsRoute } from './email-metrics.tsx'

// The section has no page of its own: it opens on its first report.
export const Route = createFileRoute('/workspaces/$workspaceId/analytics/')({
  beforeLoad: ({ params, search }) => {
    throw redirect({ to: emailMetricsRoute.to, params, search })
  },
})
