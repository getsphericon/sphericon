import { createLazyFileRoute } from '@tanstack/react-router'

import { RuleCreatePage } from '../../../../pages/rules/resource.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/rules/new')({
  component: RuleCreatePage,
})
