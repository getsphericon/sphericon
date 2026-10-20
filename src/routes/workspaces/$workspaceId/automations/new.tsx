import { createFileRoute } from '@tanstack/react-router'

import { AutomationCreatePage } from '../../../../pages/automations/create.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/automations/new')({
  component: AutomationCreatePage,
})
