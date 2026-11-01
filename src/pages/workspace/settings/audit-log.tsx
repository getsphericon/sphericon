import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Route as auditLogRoute } from '../../../routes/workspaces/$workspaceId/settings/audit-log.tsx'
import type { AuditFilter } from '../../../utils/auditFilter.ts'
import { can } from '../../../utils/permissions.ts'
import { AuditExportButton, AuditLogSection } from '../AuditLogSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

// The log is read with audit:read, so anyone without it sees the title and no section; its
// retention is set with audit:configure.
export function AuditLogPage() {
  const { t } = useTranslation()
  const { workspaceId } = auditLogRoute.useParams()
  const navigate = useNavigate()
  // The Audit log filter rides in the route's search, validated by the route.
  const auditFilter = auditLogRoute.useSearch()
  const setAuditFilter = (filter: AuditFilter) =>
    void navigate({ to: auditLogRoute.to, params: { workspaceId }, search: filter })

  return (
    <SettingsFrame
      workspaceId={workspaceId}
      title={t(($) => $.settings.nav.auditLog)}
      renderActions={(workspace) =>
        can(workspace, 'audit:read') ? (
          <AuditExportButton workspaceId={workspace.id} filter={auditFilter} />
        ) : null
      }
    >
      {(workspace) =>
        can(workspace, 'audit:read') ? (
          <AuditLogSection
            workspaceId={workspace.id}
            filter={auditFilter}
            onFilterChange={setAuditFilter}
            canConfigureRetention={can(workspace, 'audit:configure')}
          />
        ) : null
      }
    </SettingsFrame>
  )
}
