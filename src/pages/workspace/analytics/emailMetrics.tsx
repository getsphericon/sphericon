import { AreaChart, DonutChart } from '@mantine/charts'
import { EmptyState, SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { IconChartBar } from '@tabler/icons-react'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { StatCard } from '../../../components/StatCard.tsx'
import { siteAnalyticsOverviewOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import { useAnalyticsRange } from '../../../hooks/useAnalyticsRange.ts'
import { Route as emailMetricsRoute } from '../../../routes/workspaces/$workspaceId/analytics/email-metrics.tsx'
import { ReportBoundary } from './ReportBoundary.tsx'

// EmailMetricsPage is the Email metrics report: KPI cards and charts for the section's window.
export function EmailMetricsPage() {
  return (
    <ReportBoundary>
      <EmailMetricsReport />
    </ReportBoundary>
  )
}

function EmailMetricsReport() {
  const { t } = useTranslation()
  const { workspaceId } = emailMetricsRoute.useParams()
  const range = useAnalyticsRange()

  const overview = useSuspenseQuery(
    siteAnalyticsOverviewOptions({ path: { workspaceId }, query: { range } }),
  ).data
  // The report shows the range itself; the period before it is the overview's to compare with.
  const { email, newContacts } = overview.current

  return (
    <Stack>
      <Stack gap="xl">
        <Stack gap="xs">
          <Title order={4}>{t(($) => $.analytics.sections.email)}</Title>
          <Text size="sm" c="dimmed">
            {t(($) => $.analytics.email.rangeNote)}
          </Text>
          <SimpleGrid cols={{ base: 2, sm: 3, lg: 6 }}>
            <StatCard label={t(($) => $.analytics.email.sent)} value={email.sentCount} />
            <StatCard label={t(($) => $.analytics.email.opened)} value={email.openedCount} />
            <StatCard label={t(($) => $.analytics.email.clicked)} value={email.clickedCount} />
            <StatCard label={t(($) => $.analytics.email.openRate)} value={email.openRate} percent />
            <StatCard
              label={t(($) => $.analytics.email.clickRate)}
              value={email.clickRate}
              percent
            />
            <StatCard
              label={t(($) => $.analytics.email.clickToOpenRate)}
              value={email.clickToOpenRate}
              percent
            />
          </SimpleGrid>
          {email.sentCount === 0 ? (
            <EmptyState icon={<IconChartBar />} title={t(($) => $.analytics.email.empty)} />
          ) : (
            <AreaChart
              h={260}
              data={overview.timeseries}
              dataKey="date"
              withLegend
              curveType="monotone"
              series={[
                { name: 'sent', label: t(($) => $.analytics.series.sent), color: 'blue.6' },
                { name: 'opened', label: t(($) => $.analytics.series.opened), color: 'teal.6' },
                {
                  name: 'clicked',
                  label: t(($) => $.analytics.series.clicked),
                  color: 'grape.6',
                },
              ]}
            />
          )}
        </Stack>

        <SimpleGrid cols={{ base: 1, md: 2 }}>
          <Stack gap="xs">
            <Title order={4}>{t(($) => $.analytics.sections.contacts)}</Title>
            <SimpleGrid cols={2}>
              <StatCard
                label={t(($) => $.analytics.contacts.total)}
                value={overview.contacts.total}
              />
              <StatCard
                label={t(($) => $.analytics.contacts.active)}
                value={overview.contacts.active}
              />
              <StatCard
                label={t(($) => $.analytics.contacts.unsubscribed)}
                value={overview.contacts.unsubscribed}
              />
              <StatCard label={t(($) => $.analytics.contacts.newInRange)} value={newContacts} />
            </SimpleGrid>
            {overview.contacts.total > 0 ? (
              <DonutChart
                h={200}
                withLabels
                data={[
                  {
                    name: t(($) => $.analytics.contacts.active),
                    value: overview.contacts.active,
                    color: 'teal.6',
                  },
                  {
                    name: t(($) => $.analytics.contacts.unsubscribed),
                    value: overview.contacts.unsubscribed,
                    color: 'gray.5',
                  },
                ]}
              />
            ) : (
              <EmptyState icon={<IconChartBar />} title={t(($) => $.analytics.contacts.empty)} />
            )}
          </Stack>

          <Stack gap="xs">
            <Title order={4}>{t(($) => $.analytics.sections.automations)}</Title>
            <SimpleGrid cols={2}>
              <StatCard
                label={t(($) => $.analytics.automations.total)}
                value={overview.automations.total}
              />
              <StatCard
                label={t(($) => $.analytics.automations.active)}
                value={overview.automations.active}
              />
              <StatCard
                label={t(($) => $.analytics.automations.runsActive)}
                value={overview.automations.runsActive}
              />
              <StatCard
                label={t(($) => $.analytics.automations.runsCompleted)}
                value={overview.automations.runsCompleted}
              />
            </SimpleGrid>
          </Stack>
        </SimpleGrid>
      </Stack>
    </Stack>
  )
}
