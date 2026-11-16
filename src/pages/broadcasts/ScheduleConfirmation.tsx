import { Button, Group, Skeleton, Stack, Text } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import {
  siteSegmentsGetOptions,
  siteSegmentsPreviewOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteBroadcastResource,
  SiteConditionGroupInput,
} from '../../generated/site/types.gen.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'

// What Schedule arms, shown before it does (ADR 0149): the Recipients the saved Broadcast would take
// now, whether or not a Send cap applies. Nothing is scheduled until the person confirms.
export function ScheduleConfirmation({
  broadcast,
  onConfirm,
  onCancel,
}: {
  broadcast: SiteBroadcastResource
  onConfirm: () => void
  onCancel: () => void
}) {
  const { t } = useTranslation()
  return (
    <Stack>
      <QueryBoundary
        fallback={<Skeleton height={40} />}
        errorTitle={t(($) => $.broadcasts.scheduleConfirm.countError)}
      >
        {broadcast.segmentId ? (
          <SegmentRecipients broadcast={broadcast} segmentId={broadcast.segmentId} />
        ) : (
          <Recipients broadcast={broadcast} />
        )}
      </QueryBoundary>
      <Group justify="flex-end">
        <Button variant="default" onClick={onCancel}>
          {t(($) => $.actions.cancel)}
        </Button>
        <Button onClick={onConfirm}>{t(($) => $.broadcasts.scheduleConfirm.confirm)}</Button>
      </Group>
    </Stack>
  )
}

// A Broadcast to a Segment is counted by the Segment's condition as it is now: the one Schedule
// copies.
function SegmentRecipients({
  broadcast,
  segmentId,
}: {
  broadcast: SiteBroadcastResource
  segmentId: string
}) {
  const { workspaceId } = workspaceRoute.useParams()
  const { data: segment } = useSuspenseQuery(
    siteSegmentsGetOptions({ path: { workspaceId, id: segmentId } }),
  )
  return <Recipients broadcast={broadcast} condition={segment.condition} />
}

// The count is the segment preview's recipients on the Broadcast's Channel and Integration: the
// figure Schedule answers too. It is the count at Schedule; who receives it is settled when it sends.
function Recipients({
  broadcast,
  condition,
}: {
  broadcast: SiteBroadcastResource
  condition?: SiteConditionGroupInput | undefined
}) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  const { data } = useSuspenseQuery(
    siteSegmentsPreviewOptions({
      path: { workspaceId },
      body: {
        ...(condition ? { condition } : {}),
        channel: broadcast.channel,
        ...(broadcast.integrationId ? { integrationId: broadcast.integrationId } : {}),
      },
    }),
  )
  return (
    <Text>
      {t(($) => $.broadcasts.scheduleConfirm.recipients, { count: data.recipients ?? data.count })}
    </Text>
  )
}
