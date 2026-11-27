import { Alert, Group, Stack, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { BlockSkeleton } from '../../../components/Skeletons.tsx'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { workspaceOptions } from '../../../utils/workspace.ts'

// SettingsFrame is the chrome every settings page shares: its title, with the page's actions
// beside it, then the loaded workspace handed to `children` (or the load error / spinner while
// it is not there).
export function SettingsFrame({
  workspaceId,
  title,
  renderActions,
  children,
}: {
  workspaceId: string
  title: string
  renderActions?: (workspace: SiteWorkspaceResource) => ReactNode
  children: (workspace: SiteWorkspaceResource) => ReactNode
}) {
  const { t } = useTranslation()
  const workspaceQuery = useQuery(workspaceOptions(workspaceId))
  const workspace = workspaceQuery.data

  return (
    <Stack>
      <Group justify="space-between">
        <Title order={2}>{title}</Title>
        {workspace && renderActions ? renderActions(workspace) : null}
      </Group>

      {workspaceQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.loadErrorTitle)}>
          {t(($) => $.settings.loadErrorTitle)}
        </Alert>
      ) : null}

      {workspaceQuery.isLoading ? <BlockSkeleton /> : null}

      {workspace ? children(workspace) : null}
    </Stack>
  )
}
