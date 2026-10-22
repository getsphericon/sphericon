import { createFileRoute } from '@tanstack/react-router'

import { RuleEditPage } from '../../../../../pages/rules/resource.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/rules/$ruleId/edit')({
  component: RuleEditPage,
})
