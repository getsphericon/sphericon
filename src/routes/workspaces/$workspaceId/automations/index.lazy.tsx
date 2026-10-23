import { createLazyFileRoute } from '@tanstack/react-router'

import { AutomationsListPage } from '../../../../pages/automations/list.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/automations/')({
  component: AutomationsListPage,
})
