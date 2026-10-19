import { Stack } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Route as teamRoute } from '../../../routes/workspaces/$slug/settings/team.tsx'
import type { AuditFilter } from '../auditFilter.ts'
import { AuditLogSection } from '../AuditLogSection.tsx'
import { MembersSection } from '../MembersSection.tsx'
import { SecondFactorRequirementSection } from '../SecondFactorRequirementSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function TeamPage() {
  const { t } = useTranslation()
  const { slug } = teamRoute.useParams()
  const navigate = useNavigate()
  // The Audit log filter rides in the route's search, validated by the route.
  const auditFilter = teamRoute.useSearch()
  const setAuditFilter = (filter: AuditFilter) =>
    void navigate({ to: teamRoute.to, params: { slug }, search: filter })

  return (
    <SettingsFrame slug={slug} title={t(($) => $.settings.nav.team)}>
      {(workspace) => (
        <Stack>
          <MembersSection slug={workspace.slug} />
          <SecondFactorRequirementSection workspace={workspace} />
          <AuditLogSection
            slug={workspace.slug}
            filter={auditFilter}
            onFilterChange={setAuditFilter}
          />
        </Stack>
      )}
    </SettingsFrame>
  )
}
