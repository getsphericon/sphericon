import { createFileRoute } from '@tanstack/react-router'

import { AutomationEditPage } from '../../../../../pages/automations/edit.tsx'

export const Route = createFileRoute('/workspaces/$slug/automations/$automationId/edit')({
  component: AutomationEditPage,
})
