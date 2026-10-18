import { createFileRoute } from '@tanstack/react-router'

import { auditFilterSchema } from '../../../pages/workspace/auditFilter.ts'
import { SettingsPage } from '../../../pages/workspace/settings.tsx'

export const Route = createFileRoute('/workspaces/$slug/settings')({
  // The Audit log filter, so a Change history link opens the log pre-filtered.
  validateSearch: auditFilterSchema,
  component: SettingsPage,
})
