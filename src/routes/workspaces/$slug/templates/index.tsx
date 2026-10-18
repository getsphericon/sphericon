import { createFileRoute } from '@tanstack/react-router'

import { TemplatesListPage } from '../../../../pages/templates/list.tsx'

export const Route = createFileRoute('/workspaces/$slug/templates/')({
  component: TemplatesListPage,
})
