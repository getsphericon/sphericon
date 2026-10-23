import { Stack } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { canManageJoinDomains } from '../../../hooks/useCurrentRole.ts'
import { Route as teamRoute } from '../../../routes/workspaces/$workspaceId/settings/team.tsx'
import { JoinDomainsSection } from '../JoinDomainsSection.tsx'
import { MembersSection } from '../MembersSection.tsx'
import { SecondFactorRequirementSection } from '../SecondFactorRequirementSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function TeamPage() {
  const { t } = useTranslation()
  const { workspaceId } = teamRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.team)}>
      {(workspace) => (
        <Stack gap="xl">
          <MembersSection workspaceId={workspace.id} />
          <SecondFactorRequirementSection workspace={workspace} />
          {canManageJoinDomains(workspace.role) ? (
            <JoinDomainsSection workspaceId={workspace.id} />
          ) : null}
        </Stack>
      )}
    </SettingsFrame>
  )
}
