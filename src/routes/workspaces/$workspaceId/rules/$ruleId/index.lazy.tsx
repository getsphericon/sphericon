import { createLazyFileRoute } from '@tanstack/react-router'

import { RuleDetailPage } from '../../../../../pages/rules/detail.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/rules/$ruleId/')({
  component: RuleDetailPage,
})
