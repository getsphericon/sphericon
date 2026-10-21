import { Stack } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { canManageJoinDomains } from '../../../hooks/useCurrentRole.ts'
import { Route as teamRoute } from '../../../routes/workspaces/$workspaceId/settings/team.tsx'
import type { AuditFilter } from '../auditFilter.ts'
import { AuditLogSection } from '../AuditLogSection.tsx'
import { JoinDomainsSection } from '../JoinDomainsSection.tsx'
import { MembersSection } from '../MembersSection.tsx'
import { SecondFactorRequirementSection } from '../SecondFactorRequirementSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function TeamPage() {
  const { t } = useTranslation()
  const { workspaceId } = teamRoute.useParams()
  const navigate = useNavigate()
  // The Audit log filter rides in the route's search, validated by the route.
  const auditFilter = teamRoute.useSearch()
  const setAuditFilter = (filter: AuditFilter) =>
    void navigate({ to: teamRoute.to, params: { workspaceId }, search: filter })

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.team)}>
      {(workspace) => (
        <Stack>
          <MembersSection workspaceId={workspace.id} />
          <SecondFactorRequirementSection workspace={workspace} />
          {canManageJoinDomains(workspace.role) ? (
            <JoinDomainsSection workspaceId={workspace.id} />
          ) : null}
          <AuditLogSection
            workspaceId={workspace.id}
            filter={auditFilter}
            onFilterChange={setAuditFilter}
          />
        </Stack>
      )}
    </SettingsFrame>
  )
}
