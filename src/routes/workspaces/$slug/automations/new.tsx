import { createFileRoute } from '@tanstack/react-router'

import { AutomationCreatePage } from '../../../../pages/automations/create.tsx'

export const Route = createFileRoute('/workspaces/$slug/automations/new')({
  component: AutomationCreatePage,
})
