import { createLazyFileRoute } from '@tanstack/react-router'

import { RuleEditPage } from '../../../../../pages/rules/resource.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/rules/$ruleId/edit')({
  component: RuleEditPage,
})
