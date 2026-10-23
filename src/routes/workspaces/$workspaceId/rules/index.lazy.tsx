import { createLazyFileRoute } from '@tanstack/react-router'

import { RulesListPage } from '../../../../pages/rules/list.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/rules/')({
  component: RulesListPage,
})
