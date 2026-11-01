import { Card, Group, SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { useSuspenseQueries } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { AnchorLink, ButtonLink } from '../../components/RouterLink.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import {
  siteAnalyticsOverviewOptions,
  siteBroadcastsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteBroadcastResource } from '../../generated/site/types.gen.ts'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { Route as automationsRoute } from '../../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as broadcastReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { Route as broadcastsRoute } from '../../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$workspaceId/broadcasts/new.tsx'
import { formatPercent } from '../../utils/percent.ts'

const LIMIT = 5

// BroadcastLine is one Broadcast of a list: its name, linked to its report, and a line under it.
function BroadcastLine({
  workspaceId,
  broadcast,
  detail,
}: {
  workspaceId: string
  broadcast: SiteBroadcastResource
  detail: string
}) {
  return (
    <Stack gap={0}>
      <AnchorLink
        to={broadcastReportRoute.to}
        params={{ workspaceId, broadcastId: broadcast.id }}
        size="sm"
        fw={500}
      >
        {broadcast.name}
      </AnchorLink>
      <Text size="xs" c="dimmed">
        {detail}
      </Text>
    </Stack>
  )
}

// BroadcastColumn is one list of Broadcasts under its title, or the line that says it is empty.
function BroadcastColumn({
  workspaceId,
  title,
  empty,
  items,
  detail,
  children,
}: {
  workspaceId: string
  title: string
  empty: string
  items: SiteBroadcastResource[]
  detail: (b: SiteBroadcastResource) => string
  children?: ReactNode
}) {
  return (
    <Stack gap="xs">
      <Text size="sm" fw={600}>
        {title}
      </Text>
      {items.length === 0 ? (
        <Text size="sm" c="dimmed">
          {empty}
        </Text>
      ) : (
        items.map((b) => (
          <BroadcastLine key={b.id} workspaceId={workspaceId} broadcast={b} detail={detail(b)} />
        ))
      )}
      {children}
    </Stack>
  )
}

// SendsLists is what is going out and what went out last: the Broadcasts sending or scheduled,
// soonest first, the last ones sent with their open rate, and how many Automations are live. A
// Workspace that has none of them yet gets one line instead of three empty lists. The Automations
// snapshot does not depend on the page's period, so it is read for the default one, once.
function SendsLists({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const { formatDateTime } = useDateFormat()
  const [upcoming, sent, overview] = useSuspenseQueries({
    queries: [
      siteBroadcastsListOptions({
        path: { workspaceId },
        query: { status: ['sending', 'scheduled'], order: 'scheduledSoonest', pageSize: LIMIT },
      }),
      siteBroadcastsListOptions({
        path: { workspaceId },
        query: { status: ['sent'], order: 'sentLatest', pageSize: LIMIT },
      }),
      siteAnalyticsOverviewOptions({ path: { workspaceId } }),
    ],
  })
  const automations = overview.data.automations
  if (upcoming.data.items.length === 0 && sent.data.items.length === 0 && automations.total === 0) {
    return (
      <Text size="sm" c="dimmed">
        {t(($) => $.overview.sends.empty)}
      </Text>
    )
  }

  const upcomingDetail = (b: SiteBroadcastResource) =>
    b.status === 'sending'
      ? t(($) => $.overview.sends.sending, {
          sent: b.stats.sentCount,
          total: b.stats.recipientsTotal,
        })
      : t(($) => $.overview.sends.scheduledFor, {
          date: b.scheduledAt ? formatDateTime(b.scheduledAt) : '',
        })
  const sentDetail = (b: SiteBroadcastResource) =>
    t(($) => $.overview.sends.sentDetail, {
      date: b.sentAt ? formatDateTime(b.sentAt) : '',
      opened: formatPercent(b.stats.openRate),
      clicked: formatPercent(b.stats.clickRate),
    })

  return (
    <SimpleGrid cols={{ base: 1, md: 3 }}>
      <BroadcastColumn
        workspaceId={workspaceId}
        title={t(($) => $.overview.sends.upcomingTitle)}
        empty={t(($) => $.overview.sends.upcomingEmpty)}
        items={upcoming.data.items}
        detail={upcomingDetail}
      />
      <BroadcastColumn
        workspaceId={workspaceId}
        title={t(($) => $.overview.sends.lastSentTitle)}
        empty={t(($) => $.overview.sends.lastSentEmpty)}
        items={sent.data.items}
        detail={sentDetail}
      >
        <AnchorLink to={broadcastsRoute.to} params={{ workspaceId }} size="sm">
          {t(($) => $.overview.sends.allBroadcasts)}
        </AnchorLink>
      </BroadcastColumn>
      <Stack gap="xs">
        <Text size="sm" fw={600}>
          {t(($) => $.overview.sends.automationsTitle)}
        </Text>
        <Text size="sm">
          {t(($) => $.overview.sends.automations, {
            count: automations.active,
            runs: automations.runsActive,
          })}
        </Text>
        <AnchorLink to={automationsRoute.to} params={{ workspaceId }} size="sm">
          {t(($) => $.overview.sends.allAutomations)}
        </AnchorLink>
      </Stack>
    </SimpleGrid>
  )
}

// SendsBlock replaces the create cards and the list of what was created last: what is going out
// and what went out tells more than creation order, and creating is a button in its header. It
// is live, not bound to the page's period.
export function SendsBlock({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  return (
    <Card withBorder padding="lg">
      <Stack gap="md">
        <Group justify="space-between">
          <Title order={4}>{t(($) => $.overview.sends.title)}</Title>
          <Group gap="xs">
            <ButtonLink
              to={automationsRoute.to}
              params={{ workspaceId }}
              size="xs"
              variant="default"
            >
              {t(($) => $.automations.addAutomation)}
            </ButtonLink>
            <ButtonLink to={broadcastsCreateRoute.to} params={{ workspaceId }} size="xs">
              {t(($) => $.broadcasts.addBroadcast)}
            </ButtonLink>
          </Group>
        </Group>
        <QueryBoundary
          fallback={<BlockSkeleton />}
          errorTitle={t(($) => $.overview.sends.loadError)}
        >
          <SendsLists workspaceId={workspaceId} />
        </QueryBoundary>
      </Stack>
    </Card>
  )
}
