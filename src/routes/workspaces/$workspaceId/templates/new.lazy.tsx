import { createLazyFileRoute } from '@tanstack/react-router'

import { TemplateCreatePage } from '../../../../pages/templates/resource.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/templates/new')({
  component: TemplateCreatePage,
})
