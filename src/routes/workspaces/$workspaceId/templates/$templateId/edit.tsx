import { createFileRoute } from '@tanstack/react-router'

import { TemplateEditPage } from '../../../../../pages/templates/resource.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/templates/$templateId/edit')({
  component: TemplateEditPage,
})
