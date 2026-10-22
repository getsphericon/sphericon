import { createFileRoute } from '@tanstack/react-router'

import { RuleCreatePage } from '../../../../pages/rules/resource.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/rules/new')({
  component: RuleCreatePage,
})
