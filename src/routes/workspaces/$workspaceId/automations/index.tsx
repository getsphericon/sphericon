import { createFileRoute } from '@tanstack/react-router'

import { AutomationsListPage } from '../../../../pages/automations/list.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/automations/')({
  component: AutomationsListPage,
})
