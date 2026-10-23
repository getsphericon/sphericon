import { createLazyFileRoute } from '@tanstack/react-router'

import { AutomationDetailPage } from '../../../../../pages/automations/detail.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/automations/$automationId/')({
  component: AutomationDetailPage,
})
