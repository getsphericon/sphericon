import { createLazyFileRoute } from '@tanstack/react-router'

import { OpensByHourPage } from '../../../../pages/workspace/analytics/opensByHour.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/analytics/opens-by-hour')({
  component: OpensByHourPage,
})
