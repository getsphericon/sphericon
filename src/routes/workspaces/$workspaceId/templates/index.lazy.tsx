import { createLazyFileRoute } from '@tanstack/react-router'

import { TemplatesListPage } from '../../../../pages/templates/list.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/templates/')({
  component: TemplatesListPage,
})
