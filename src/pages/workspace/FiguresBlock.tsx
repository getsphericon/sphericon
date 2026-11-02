import { Card, Group, SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { AnchorLink } from '../../components/RouterLink.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import { StatCard } from '../../components/StatCard.tsx'
import { siteAnalyticsOverviewOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteAnalyticsRange, SiteAnalyticsSending } from '../../generated/site/types.gen.ts'
import { Route as emailMetricsRoute } from '../../routes/workspaces/$workspaceId/analytics/email-metrics.tsx'
import { Route as subscribedPeopleRoute } from '../../routes/workspaces/$workspaceId/analytics/subscribed-people.tsx'
import { formatChange, formatPointChange } from '../../utils/change.ts'

interface FiguresProps {
  workspaceId: string
  range: SiteAnalyticsRange
  // periodControl picks the period; it is shown above the cards, and not at all when there is no
  // card, so a new Workspace sees no control over nothing.
  periodControl: ReactNode
}

// sentIn is every message a period sent, through every surface.
const sentIn = (sending: SiteAnalyticsSending) =>
  sending.broadcast + sending.automation + sending.transactional

// Figures is the period's sending and audience, each card only once it has something to show:
// sending from the first message of the period, the audience from the first Contact. A new
// Workspace sees neither, rather than a wall of zeros. Every figure of the period has the previous
// period under it, with the change since; the number of the subscribed is a snapshot and has none.
function Figures({ workspaceId, range, periodControl }: FiguresProps) {
  const { t } = useTranslation()
  const overview = useSuspenseQuery(
    siteAnalyticsOverviewOptions({ path: { workspaceId }, query: { range } }),
  ).data
  const { current, previous, contacts } = overview
  const sent = sentIn(current.sending)
  if (sent === 0 && contacts.total === 0) {
    return null
  }

  const label = t(($) => $.overview.previous[range])
  // compared is the line under a count: its value in the previous period and the change since.
  const compared = (now: number, before: number) => ({
    label,
    value: before,
    change: formatChange(now, before),
  })
  // comparedRate is the line under an engagement rate: its change is in percentage points, and
  // there is none when the previous period sent no Broadcast message to have a rate of.
  const comparedRate = (now: number, before: number) => ({
    label,
    value: before,
    change:
      previous.email.sentCount === 0
        ? null
        : t(($) => $.overview.pointChange, { change: formatPointChange(now, before) }),
  })

  return (
    <>
      {periodControl}
      {sent > 0 && (
        <Card withBorder padding="lg">
          <Stack gap="sm">
            <Group justify="space-between">
              <Title order={4}>{t(($) => $.overview.sending.title)}</Title>
              <AnchorLink
                to={emailMetricsRoute.to}
                params={{ workspaceId }}
                search={{ range }}
                size="sm"
              >
                {t(($) => $.overview.sending.report)}
              </AnchorLink>
            </Group>
            <SimpleGrid cols={{ base: 2, sm: 4 }}>
              <StatCard
                label={t(($) => $.overview.sending.sent)}
                value={sent}
                comparison={compared(sent, sentIn(previous.sending))}
              />
              <StatCard
                label={t(($) => $.overview.sending.delivered)}
                value={current.sending.delivered}
                comparison={compared(current.sending.delivered, previous.sending.delivered)}
              />
              <StatCard
                label={t(($) => $.overview.sending.bounced)}
                value={current.sending.bounced}
                comparison={compared(current.sending.bounced, previous.sending.bounced)}
              />
              <StatCard
                label={t(($) => $.overview.sending.complained)}
                value={current.sending.complained}
                comparison={compared(current.sending.complained, previous.sending.complained)}
              />
            </SimpleGrid>
            <Text size="xs" c="dimmed">
              {t(($) => $.overview.sending.split, current.sending)}
            </Text>
          </Stack>
        </Card>
      )}
      {contacts.total > 0 && (
        <Card withBorder padding="lg">
          <Stack gap="sm">
            <Group justify="space-between">
              <Title order={4}>{t(($) => $.overview.audience.title)}</Title>
              <AnchorLink
                to={subscribedPeopleRoute.to}
                params={{ workspaceId }}
                search={{ range }}
                size="sm"
              >
                {t(($) => $.overview.audience.report)}
              </AnchorLink>
            </Group>
            <SimpleGrid cols={{ base: 2, sm: 4 }}>
              <StatCard label={t(($) => $.overview.audience.subscribed)} value={contacts.active} />
              <StatCard
                label={t(($) => $.overview.audience.new)}
                value={current.newContacts}
                comparison={compared(current.newContacts, previous.newContacts)}
              />
              <StatCard
                label={t(($) => $.overview.audience.openRate)}
                value={current.email.openRate}
                percent
                comparison={comparedRate(current.email.openRate, previous.email.openRate)}
              />
              <StatCard
                label={t(($) => $.overview.audience.clickRate)}
                value={current.email.clickRate}
                percent
                comparison={comparedRate(current.email.clickRate, previous.email.clickRate)}
              />
            </SimpleGrid>
          </Stack>
        </Card>
      )}
    </>
  )
}

// FiguresBlock is the overview's sending and audience for the page's period; a new period starts
// over, so a failed one does not stay on screen.
export function FiguresBlock({ workspaceId, range, periodControl }: FiguresProps) {
  const { t } = useTranslation()
  return (
    <QueryBoundary
      key={range}
      fallback={<BlockSkeleton />}
      errorTitle={t(($) => $.overview.figuresLoadError)}
    >
      <Figures workspaceId={workspaceId} range={range} periodControl={periodControl} />
    </QueryBoundary>
  )
}
