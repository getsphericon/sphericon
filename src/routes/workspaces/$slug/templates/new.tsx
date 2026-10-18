import { createFileRoute } from '@tanstack/react-router'

import { TemplateCreatePage } from '../../../../pages/templates/resource.tsx'

export const Route = createFileRoute('/workspaces/$slug/templates/new')({
  component: TemplateCreatePage,
})
