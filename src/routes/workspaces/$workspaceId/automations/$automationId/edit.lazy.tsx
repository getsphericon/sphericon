import { createLazyFileRoute } from '@tanstack/react-router'

import { AutomationEditPage } from '../../../../../pages/automations/edit.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/automations/$automationId/edit')(
  {
    component: AutomationEditPage,
  },
)
