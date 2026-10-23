import { Alert, Stack, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { BlockSkeleton } from '../../../components/Skeletons.tsx'
import { siteWorkspacesListOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'

// SettingsFrame is the chrome every settings page shares: its title, then the loaded
// workspace handed to `children` (or the load error / spinner while it is not there).
export function SettingsFrame({
  workspaceId,
  title,
  children,
}: {
  workspaceId: string
  title: string
  children: (workspace: SiteWorkspaceResource) => ReactNode
}) {
  const { t } = useTranslation()
  const workspacesQuery = useQuery(siteWorkspacesListOptions())
  const workspace = workspacesQuery.data?.find((w) => w.id === workspaceId)

  return (
    <Stack>
      <Title order={2}>{title}</Title>

      {workspacesQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.loadErrorTitle)}>
          {t(($) => $.settings.loadErrorTitle)}
        </Alert>
      ) : null}

      {workspacesQuery.isLoading ? <BlockSkeleton /> : null}

      {workspace ? children(workspace) : null}
    </Stack>
  )
}
