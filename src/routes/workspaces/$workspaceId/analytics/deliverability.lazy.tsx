import { createLazyFileRoute } from '@tanstack/react-router'

import { DeliverabilityPage } from '../../../../pages/workspace/analytics/deliverability.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/analytics/deliverability')({
  component: DeliverabilityPage,
})
