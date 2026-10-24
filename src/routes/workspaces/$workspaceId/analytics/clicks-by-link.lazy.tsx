import { createLazyFileRoute } from '@tanstack/react-router'

import { ClicksByLinkPage } from '../../../../pages/workspace/analytics/clicksByLink.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/analytics/clicks-by-link')({
  component: ClicksByLinkPage,
})
