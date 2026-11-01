import { Card, Group, SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { AnchorLink } from '../../components/RouterLink.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import { StatCard } from '../../components/StatCard.tsx'
import { siteAnalyticsOverviewOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteAnalyticsRange } from '../../generated/site/types.gen.ts'
import { Route as emailMetricsRoute } from '../../routes/workspaces/$workspaceId/analytics/email-metrics.tsx'
import { Route as subscribedPeopleRoute } from '../../routes/workspaces/$workspaceId/analytics/subscribed-people.tsx'

interface FiguresProps {
  workspaceId: string
  range: SiteAnalyticsRange
}

// Figures is the period's sending and audience, each card only once it has something to show:
// sending from the first message of the period, the audience from the first Contact. A new
// Workspace sees neither, rather than a wall of zeros.
function Figures({ workspaceId, range }: FiguresProps) {
  const { t } = useTranslation()
  const overview = useSuspenseQuery(
    siteAnalyticsOverviewOptions({ path: { workspaceId }, query: { range } }),
  ).data
  const { sending, email, contacts } = overview
  const sent = sending.broadcast + sending.automation + sending.transactional

  return (
    <>
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
              <StatCard label={t(($) => $.overview.sending.sent)} value={sent} />
              <StatCard label={t(($) => $.overview.sending.delivered)} value={sending.delivered} />
              <StatCard label={t(($) => $.overview.sending.bounced)} value={sending.bounced} />
              <StatCard
                label={t(($) => $.overview.sending.complained)}
                value={sending.complained}
              />
            </SimpleGrid>
            <Text size="xs" c="dimmed">
              {t(($) => $.overview.sending.split, sending)}
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
              <StatCard label={t(($) => $.overview.audience.new)} value={contacts.newInRange} />
              <StatCard
                label={t(($) => $.overview.audience.openRate)}
                value={email.openRate}
                percent
              />
              <StatCard
                label={t(($) => $.overview.audience.clickRate)}
                value={email.clickRate}
                percent
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
export function FiguresBlock({ workspaceId, range }: FiguresProps) {
  const { t } = useTranslation()
  return (
    <QueryBoundary
      key={range}
      fallback={<BlockSkeleton />}
      errorTitle={t(($) => $.overview.figuresLoadError)}
    >
      <Figures workspaceId={workspaceId} range={range} />
    </QueryBoundary>
  )
}
