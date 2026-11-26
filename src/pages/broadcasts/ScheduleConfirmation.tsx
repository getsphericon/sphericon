import { Button, Group, Skeleton, Stack, Text } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import {
  siteBroadcastsOutsideWindowOptions,
  siteSegmentsGetOptions,
  siteSegmentsPreviewOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteBroadcastResource,
  SegmentConditionInput,
  SiteBroadcastOutsideWindowInput,
} from '../../generated/site/types.gen.ts'
import { useFormat } from '../../hooks/useFormat.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'

// What Schedule arms, shown before it does (ADR 0149): the Recipients the saved Broadcast would take
// now, whether or not a Send cap applies, and on SMS how many of them the Sending window would hold
// at scheduledAt, the ISO moment chosen (ADR 0134). Nothing is scheduled until the person confirms.
export function ScheduleConfirmation({
  broadcast,
  scheduledAt,
  onConfirm,
  onCancel,
}: {
  broadcast: SiteBroadcastResource
  scheduledAt: string
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
          <SegmentRecipients
            broadcast={broadcast}
            segmentId={broadcast.segmentId}
            scheduledAt={scheduledAt}
          />
        ) : (
          <Recipients broadcast={broadcast} scheduledAt={scheduledAt} />
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
  scheduledAt,
}: {
  broadcast: SiteBroadcastResource
  segmentId: string
  scheduledAt: string
}) {
  const { workspaceId } = workspaceRoute.useParams()
  const { data: segment } = useSuspenseQuery(
    siteSegmentsGetOptions({ path: { workspaceId, id: segmentId } }),
  )
  return (
    <Recipients broadcast={broadcast} condition={segment.condition} scheduledAt={scheduledAt} />
  )
}

// The count is the segment preview's recipients on the Broadcast's Channel and Integration: the
// figure Schedule answers too. It is the count at Schedule; who receives it is settled when it sends.
function Recipients({
  broadcast,
  condition,
  scheduledAt,
}: {
  broadcast: SiteBroadcastResource
  condition?: SegmentConditionInput | undefined
  scheduledAt: string
}) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  const audience = {
    ...(condition ? { condition } : {}),
    channel: broadcast.channel,
    ...(broadcast.integrationId ? { integrationId: broadcast.integrationId } : {}),
  }
  const { data } = useSuspenseQuery(
    siteSegmentsPreviewOptions({ path: { workspaceId }, body: audience }),
  )
  const recipients = data.recipients ?? data.count
  return (
    <>
      <Text>{t(($) => $.broadcasts.scheduleConfirm.recipients, { count: recipients })}</Text>
      {broadcast.channel === 'sms' && (
        <QueryBoundary
          fallback={<Skeleton height={24} />}
          errorTitle={t(($) => $.broadcasts.scheduleConfirm.outsideWindowError)}
        >
          <OutsideWindow audience={audience} scheduledAt={scheduledAt} recipients={recipients} />
        </QueryBoundary>
      )}
    </>
  )
}

// The share of the recipients the Sending window would hold at the moment chosen, asked of the same
// audience as the count, and when the last of them is reached; nothing while none is held.
function OutsideWindow({
  audience,
  scheduledAt,
  recipients,
}: {
  audience: Omit<SiteBroadcastOutsideWindowInput, 'scheduledAt'>
  scheduledAt: string
  recipients: number
}) {
  const { t } = useTranslation()
  const { formatDateTime, formatPercent } = useFormat()
  const { workspaceId } = workspaceRoute.useParams()
  const { data } = useSuspenseQuery(
    siteBroadcastsOutsideWindowOptions({
      path: { workspaceId },
      body: { ...audience, scheduledAt },
    }),
  )
  if (data.held === 0 || data.reachedAt === undefined) {
    return null
  }
  return (
    <Text>
      {t(($) => $.broadcasts.scheduleConfirm.outsideWindow, {
        count: data.held,
        share: formatPercent(recipients > 0 ? data.held / recipients : 1),
        reachedAt: formatDateTime(data.reachedAt),
      })}
    </Text>
  )
}
