import { createFileRoute } from '@tanstack/react-router'

import { auditFilterSchema } from '../../../../pages/workspace/auditFilter.ts'

export const Route = createFileRoute('/workspaces/$workspaceId/settings/audit-log')({
  // The Audit log filter, so a Change history link opens the log pre-filtered.
  validateSearch: auditFilterSchema,
})
