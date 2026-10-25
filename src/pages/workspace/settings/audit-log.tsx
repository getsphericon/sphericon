import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Route as auditLogRoute } from '../../../routes/workspaces/$workspaceId/settings/audit-log.tsx'
import type { AuditFilter } from '../../../utils/auditFilter.ts'
import { AuditLogSection } from '../AuditLogSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function AuditLogPage() {
  const { t } = useTranslation()
  const { workspaceId } = auditLogRoute.useParams()
  const navigate = useNavigate()
  // The Audit log filter rides in the route's search, validated by the route.
  const auditFilter = auditLogRoute.useSearch()
  const setAuditFilter = (filter: AuditFilter) =>
    void navigate({ to: auditLogRoute.to, params: { workspaceId }, search: filter })

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.auditLog)}>
      {(workspace) => (
        <AuditLogSection
          workspaceId={workspace.id}
          filter={auditFilter}
          onFilterChange={setAuditFilter}
        />
      )}
    </SettingsFrame>
  )
}
