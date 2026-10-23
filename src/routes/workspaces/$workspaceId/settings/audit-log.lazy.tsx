import { createLazyFileRoute } from '@tanstack/react-router'

import { AuditLogPage } from '../../../../pages/workspace/settings/audit-log.tsx'

export const Route = createLazyFileRoute('/workspaces/$workspaceId/settings/audit-log')({
  component: AuditLogPage,
})
