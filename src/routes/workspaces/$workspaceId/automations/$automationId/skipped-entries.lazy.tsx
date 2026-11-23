import { createLazyFileRoute } from '@tanstack/react-router'

import { SkippedEntriesPage } from '../../../../../pages/automations/SkippedEntries.tsx'

export const Route = createLazyFileRoute(
  '/workspaces/$workspaceId/automations/$automationId/skipped-entries',
)({
  component: SkippedEntriesPage,
})
