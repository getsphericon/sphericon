import { createLazyFileRoute } from '@tanstack/react-router'

import { EventsReportPage } from '../../../../pages/workspace/analytics/eventsReport.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/analytics/events')({
  component: EventsReportPage,
})
