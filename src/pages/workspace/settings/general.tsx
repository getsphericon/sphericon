import { Fieldset, Stack } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../../../components/Skeletons.tsx'
import { siteWorkspacesGetOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { Route as generalRoute } from '../../../routes/workspaces/$workspaceId/settings/index.tsx'
import { can } from '../../../utils/permissions.ts'
import { DangerZoneSection } from './DangerZoneSection.tsx'
import { RevenueSettingsSection } from './RevenueSettingsSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'
import { GeneralEditForm, TimezoneEditForm } from './workspaceSettingsEditors.ts'
import { CanEdit } from './workspaceSettingsForm.ts'

// The General and Time zone sections are each an editor session over the Workspace as stored
// (ADR 0119), read on its own; anyone without the Permission sees the values read-only.
function SettingsSection({
  workspace,
  permission,
  children,
}: {
  workspace: SiteWorkspaceResource
  permission: 'workspace:update' | 'workspace:timezone'
  children: (stored: SiteWorkspaceResource) => ReactNode
}) {
  const { data } = useSuspenseQuery(
    siteWorkspacesGetOptions({ path: { workspaceId: workspace.id } }),
  )
  const canEdit = can(workspace, permission)
  return (
    <CanEdit value={canEdit}>
      <Fieldset variant="unstyled" disabled={!canEdit}>
        {children(data)}
      </Fieldset>
    </CanEdit>
  )
}

function GeneralSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  // Renaming needs workspace:update (ADR 0043).
  return (
    <SettingsSection workspace={workspace} permission="workspace:update">
      {(stored) => (
        <GeneralEditForm workspaceId={workspace.id} id="general" loaded={stored} titled={false} />
      )}
    </SettingsSection>
  )
}

// TimezoneSection sets the Workspace time zone (ADR 0057): the zone every report counts its days
// in. Changing it needs workspace:timezone.
function TimezoneSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  return (
    <SettingsSection workspace={workspace} permission="workspace:timezone">
      {(stored) => (
        <TimezoneEditForm workspaceId={workspace.id} id="timezone" loaded={stored} titled={false} />
      )}
    </SettingsSection>
  )
}

export function GeneralPage() {
  const { t } = useTranslation()
  const { workspaceId } = generalRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.general)}>
      {(workspace) => (
        <Stack gap="xl">
          <QueryBoundary fallback={<BlockSkeleton />} errorTitle={t(($) => $.settings.errorTitle)}>
            <GeneralSection workspace={workspace} />
            <TimezoneSection workspace={workspace} />
          </QueryBoundary>
          <RevenueSettingsSection workspace={workspace} />
          <DangerZoneSection workspace={workspace} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
