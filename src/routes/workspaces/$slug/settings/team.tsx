import { createFileRoute } from '@tanstack/react-router'

import { auditFilterSchema } from '../../../../pages/workspace/auditFilter.ts'
import { TeamPage } from '../../../../pages/workspace/settings/team.tsx'

export const Route = createFileRoute('/workspaces/$slug/settings/team')({
  // The Audit log filter, so a Change history link opens the log pre-filtered.
  validateSearch: auditFilterSchema,
  component: TeamPage,
})
