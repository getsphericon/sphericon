import { createFileRoute } from '@tanstack/react-router'

import { RuleDetailPage } from '../../../../../pages/rules/detail.tsx'

export const Route = createFileRoute('/workspaces/$workspaceId/rules/$ruleId/')({
  component: RuleDetailPage,
})
