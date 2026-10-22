import { createFileRoute } from '@tanstack/react-router'

import { AutomationDetailPage } from '../../../../../pages/automations/detail.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/automations/$automationId/')({
  component: AutomationDetailPage,
})
