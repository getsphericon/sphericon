import { Button, Group, Stack, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
import { useConfirmation } from '../../../hooks/useConfirmation.tsx'
import { useSetEngagementTrackingDefault } from '../../../hooks/useEngagementTrackingDefault.ts'
import { can } from '../../../utils/permissions.ts'

// EngagementTrackingDefaultSection holds the Workspace's Engagement tracking default (ADR 0120):
// whether a Destination with no tracking consent recorded has its opens and clicks tracked. Every
// member sees it; an Owner or Admin changes it, after a confirmation that says what changes.
export function EngagementTrackingDefaultSection({
  workspace,
}: {
  workspace: SiteWorkspaceResource
}) {
  const { t } = useTranslation()
  const confirm = useConfirmation()
  const mutation = useSetEngagementTrackingDefault(workspace.id)
  const tracks = workspace.engagementTrackingDefault === 'track'
  const change = () =>
    confirm({
      title: tracks
        ? t(($) => $.settings.engagementTrackingDefault.toDontTrackTitle)
        : t(($) => $.settings.engagementTrackingDefault.toTrackTitle),
      description: tracks
        ? t(($) => $.settings.engagementTrackingDefault.toDontTrackDescription)
        : t(($) => $.settings.engagementTrackingDefault.toTrackDescription),
      confirmLabel: t(($) => $.settings.engagementTrackingDefault.confirm),
      onConfirm: () =>
        mutation.mutate({
          path: { workspaceId: workspace.id },
          body: {
            revision: workspace.revision,
            engagementTrackingDefault: tracks ? 'dont_track' : 'track',
          },
        }),
    })

  return (
    <Stack gap="xs">
      <Title order={4}>{t(($) => $.settings.engagementTrackingDefault.title)}</Title>
      <Text size="sm" c="dimmed">
        {t(($) => $.settings.engagementTrackingDefault.description)}
      </Text>
      <Text size="sm">
        {tracks
          ? t(($) => $.settings.engagementTrackingDefault.track)
          : t(($) => $.settings.engagementTrackingDefault.dontTrack)}
      </Text>
      {can(workspace, 'workspace:update') ? (
        <Group>
          <Button variant="light" loading={mutation.isPending} onClick={change}>
            {tracks
              ? t(($) => $.settings.engagementTrackingDefault.changeToDontTrack)
              : t(($) => $.settings.engagementTrackingDefault.changeToTrack)}
          </Button>
        </Group>
      ) : null}
    </Stack>
  )
}
