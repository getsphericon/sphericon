import { createLazyFileRoute } from '@tanstack/react-router'

import { BroadcastReportPage } from '../../../../../pages/broadcasts/report.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/broadcasts/$broadcastId/report')(
  {
    component: BroadcastReportPage,
  },
)
