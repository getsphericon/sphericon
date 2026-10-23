import { createLazyFileRoute } from '@tanstack/react-router'

import { TemplateEditPage } from '../../../../../pages/templates/resource.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/templates/$templateId/edit')({
  component: TemplateEditPage,
})
