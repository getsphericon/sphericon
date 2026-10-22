import { Badge, Group, Loader, SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { LinkClickTable } from '../../components/LinkClickTable.tsx'
import { StatCard } from '../../components/StatCard.tsx'
import {
  siteBroadcastLinksListOptions,
  siteBroadcastsGetOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteBroadcastStats, SiteBroadcastStatus } from '../../generated/site/types.gen.ts'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { BroadcastHoldAlert } from './BroadcastHold.tsx'
import { BroadcastProgress } from './BroadcastProgress.tsx'

const STATUS_COLORS: Record<SiteBroadcastStatus, string> = {
  draft: 'gray',
  scheduled: 'blue',
  sending: 'yellow',
  sent: 'teal',
  failed: 'red',
}

function statEntries(t: TFunction, stats: SiteBroadcastStats): { label: string; value: number }[] {
  return [
    { label: t(($) => $.broadcasts.stats.recipients), value: stats.recipientsTotal },
    { label: t(($) => $.broadcasts.stats.sent), value: stats.sentCount },
    { label: t(($) => $.broadcasts.stats.delivered), value: stats.deliveredCount },
    { label: t(($) => $.broadcasts.stats.bounced), value: stats.bouncedCount },
    { label: t(($) => $.broadcasts.stats.complained), value: stats.complainedCount },
    { label: t(($) => $.broadcasts.stats.opened), value: stats.openedCount },
    { label: t(($) => $.broadcasts.stats.clicked), value: stats.clickedCount },
    { label: t(($) => $.broadcasts.stats.unsubscribed), value: stats.unsubscribedCount },
    { label: t(($) => $.broadcasts.stats.failed), value: stats.failedCount },
  ]
}

function rateEntries(t: TFunction, stats: SiteBroadcastStats): { label: string; value: number }[] {
  return [
    { label: t(($) => $.broadcasts.stats.deliveryRate), value: stats.deliveryRate },
    { label: t(($) => $.broadcasts.stats.openRate), value: stats.openRate },
    { label: t(($) => $.broadcasts.stats.clickRate), value: stats.clickRate },
    { label: t(($) => $.broadcasts.stats.clickToOpenRate), value: stats.clickToOpenRate },
    { label: t(($) => $.broadcasts.stats.unsubscribeRate), value: stats.unsubscribeRate },
    { label: t(($) => $.broadcasts.stats.failureRate), value: stats.failureRate },
  ]
}

// The per-link figures load beside the report: while they fail or are empty, the report stays.
function BroadcastLinks({
  workspaceId,
  broadcastId,
}: {
  workspaceId: string
  broadcastId: string
}) {
  const linksQuery = useQuery(siteBroadcastLinksListOptions({ path: { workspaceId, broadcastId } }))
  return <LinkClickTable links={linksQuery.data?.items ?? []} />
}

export function BroadcastReportPage() {
  const { t } = useTranslation()
  const { workspaceId, broadcastId } = broadcastsReportRoute.useParams()

  const getQuery = useQuery(siteBroadcastsGetOptions({ path: { workspaceId, id: broadcastId } }))

  if (getQuery.isLoading) {
    return <Loader />
  }

  if (getQuery.isError || !getQuery.data) {
    return (
      <ApiErrorAlert
        error={getQuery.error}
        title={t(($) => $.alerts.broadcastLoadErrorTitle)}
        fallback={t(($) => $.alerts.broadcastLoadErrorTitle)}
      />
    )
  }

  const broadcast = getQuery.data

  return (
    <Stack>
      <Group justify="space-between">
        <Title order={4}>{broadcast.name}</Title>
        <Badge color={STATUS_COLORS[broadcast.status]} variant="light" size="lg">
          {t(($) => $.broadcasts.status[broadcast.status])}
        </Badge>
      </Group>
      <Text c="dimmed">{broadcast.subject}</Text>
      {broadcast.holdReason && <BroadcastHoldAlert reason={broadcast.holdReason} />}
      {broadcast.progress && <BroadcastProgress progress={broadcast.progress} />}

      <SimpleGrid cols={{ base: 2, sm: 3 }}>
        {statEntries(t, broadcast.stats).map((entry) => (
          <StatCard key={entry.label} label={entry.label} value={entry.value} />
        ))}
      </SimpleGrid>

      <SimpleGrid cols={{ base: 2, sm: 3 }}>
        {rateEntries(t, broadcast.stats).map((entry) => (
          <StatCard key={entry.label} label={entry.label} value={entry.value} percent />
        ))}
      </SimpleGrid>

      <BroadcastLinks workspaceId={workspaceId} broadcastId={broadcastId} />
    </Stack>
  )
}
