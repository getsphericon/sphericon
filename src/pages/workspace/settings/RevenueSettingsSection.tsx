import { Fieldset, Stack, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../../../components/Skeletons.tsx'
import { siteWorkspacesGetRevenueSettingsOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { can } from '../../../utils/permissions.ts'
import { RevenueEditForm } from './workspaceSettingsEditors.ts'
import { CanEdit } from './workspaceSettingsForm.ts'

function RevenueSettings({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { data } = useSuspenseQuery(
    siteWorkspacesGetRevenueSettingsOptions({ path: { workspaceId: workspace.id } }),
  )
  // An Owner or Admin edits them; a Member sees them read-only (the server refuses a Member).
  const canEdit = can(workspace, 'revenue:configure')
  return (
    <CanEdit value={canEdit}>
      <Fieldset variant="unstyled" disabled={!canEdit}>
        <RevenueEditForm workspaceId={workspace.id} id="revenue" loaded={data} titled={false} />
      </Fieldset>
    </CanEdit>
  )
}

// RevenueSettingsSection holds the Workspace's Reporting currency and Attribution window
// (ADR 0064), in an editor session over the Workspace's revision (ADR 0119). Every member sees them.
export function RevenueSettingsSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  return (
    <Stack gap="xs">
      <Title order={4}>{t(($) => $.settings.revenue.title)}</Title>
      <QueryBoundary
        fallback={<BlockSkeleton />}
        errorTitle={t(($) => $.settings.revenue.loadError)}
      >
        <RevenueSettings workspace={workspace} />
      </QueryBoundary>
    </Stack>
  )
}
