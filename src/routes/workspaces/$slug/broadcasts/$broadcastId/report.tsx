import { createFileRoute } from '@tanstack/react-router'

import { BroadcastReportPage } from '../../../../../pages/broadcasts/report.tsx'

export const Route = createFileRoute('/workspaces/$slug/broadcasts/$broadcastId/report')({
  component: BroadcastReportPage,
})
