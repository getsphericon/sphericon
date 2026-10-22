import { createFileRoute } from '@tanstack/react-router'

import { RulesListPage } from '../../../../pages/rules/list.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/rules/')({
  component: RulesListPage,
})
