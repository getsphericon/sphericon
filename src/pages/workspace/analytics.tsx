import { AreaChart, DonutChart } from '@mantine/charts'
import {
  Alert,
  EmptyState,
  Loader,
  SegmentedControl,
  SimpleGrid,
  Stack,
  Text,
  Title,
} from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { StatCard } from '../../components/StatCard.tsx'
import { siteAnalyticsOverviewOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteAnalyticsRange } from '../../generated/site/types.gen.ts'
import { zSiteAnalyticsRange } from '../../generated/site/zod.gen.ts'
import { Route as analyticsRoute } from '../../routes/workspaces/$workspaceId/analytics.tsx'
import { formatPercent } from '../../utils/percent.ts'

const RANGES = zSiteAnalyticsRange.options

// AnalyticsPage is the workspace's analytics section: KPI cards and charts for a chosen range.
export function AnalyticsPage() {
  const { t } = useTranslation()
  const { workspaceId } = analyticsRoute.useParams()
  const [range, setRange] = useState<SiteAnalyticsRange>('30d')

  const analyticsQuery = useQuery(
    siteAnalyticsOverviewOptions({ path: { workspaceId }, query: { range } }),
  )

  const abuseWarning = analyticsQuery.data?.abuseWarning

  return (
    <Stack>
      <SegmentedControl
        value={range}
        onChange={(v) => setRange(v)}
        data={RANGES.map((r) => ({ value: r, label: t(($) => $.analytics.range[r]) }))}
      />

      {analyticsQuery.isLoading && <Loader />}

      {analyticsQuery.isError && (
        <ApiErrorAlert
          error={analyticsQuery.error}
          title={t(($) => $.analytics.loadError)}
          fallback={t(($) => $.analytics.loadError)}
        />
      )}

      {abuseWarning && (
        <Alert color="yellow" title={t(($) => $.analytics.abuseWarning.title)}>
          {t(($) => $.analytics.abuseWarning.description, {
            metric: t(($) => $.analytics.abuseWarning.metrics[abuseWarning.metric]),
            rate: formatPercent(abuseWarning.rate),
            domain: abuseWarning.domain,
          })}
        </Alert>
      )}

      {analyticsQuery.data && (
        <Stack gap="xl">
          <Stack gap="xs">
            <Title order={4}>{t(($) => $.analytics.sections.email)}</Title>
            <Text size="sm" c="dimmed">
              {t(($) => $.analytics.email.rangeNote)}
            </Text>
            <SimpleGrid cols={{ base: 2, sm: 3, lg: 6 }}>
              <StatCard
                label={t(($) => $.analytics.email.sent)}
                value={analyticsQuery.data.email.sentCount}
              />
              <StatCard
                label={t(($) => $.analytics.email.opened)}
                value={analyticsQuery.data.email.openedCount}
              />
              <StatCard
                label={t(($) => $.analytics.email.clicked)}
                value={analyticsQuery.data.email.clickedCount}
              />
              <StatCard
                label={t(($) => $.analytics.email.openRate)}
                value={analyticsQuery.data.email.openRate}
                percent
              />
              <StatCard
                label={t(($) => $.analytics.email.clickRate)}
                value={analyticsQuery.data.email.clickRate}
                percent
              />
              <StatCard
                label={t(($) => $.analytics.email.clickToOpenRate)}
                value={analyticsQuery.data.email.clickToOpenRate}
                percent
              />
            </SimpleGrid>
            <AreaChart
              h={260}
              data={analyticsQuery.data.timeseries}
              dataKey="date"
              withLegend
              curveType="monotone"
              series={[
                { name: 'sent', label: t(($) => $.analytics.series.sent), color: 'blue.6' },
                { name: 'opened', label: t(($) => $.analytics.series.opened), color: 'teal.6' },
                { name: 'clicked', label: t(($) => $.analytics.series.clicked), color: 'grape.6' },
              ]}
            />
          </Stack>

          <SimpleGrid cols={{ base: 1, md: 2 }}>
            <Stack gap="xs">
              <Title order={4}>{t(($) => $.analytics.sections.contacts)}</Title>
              <SimpleGrid cols={2}>
                <StatCard
                  label={t(($) => $.analytics.contacts.total)}
                  value={analyticsQuery.data.contacts.total}
                />
                <StatCard
                  label={t(($) => $.analytics.contacts.active)}
                  value={analyticsQuery.data.contacts.active}
                />
                <StatCard
                  label={t(($) => $.analytics.contacts.unsubscribed)}
                  value={analyticsQuery.data.contacts.unsubscribed}
                />
                <StatCard
                  label={t(($) => $.analytics.contacts.newInRange)}
                  value={analyticsQuery.data.contacts.newInRange}
                />
              </SimpleGrid>
              {analyticsQuery.data.contacts.total > 0 ? (
                <DonutChart
                  h={200}
                  withLabels
                  data={[
                    {
                      name: t(($) => $.analytics.contacts.active),
                      value: analyticsQuery.data.contacts.active,
                      color: 'teal.6',
                    },
                    {
                      name: t(($) => $.analytics.contacts.unsubscribed),
                      value: analyticsQuery.data.contacts.unsubscribed,
                      color: 'gray.5',
                    },
                  ]}
                />
              ) : (
                <EmptyState title={t(($) => $.analytics.contacts.empty)} />
              )}
            </Stack>

            <Stack gap="xs">
              <Title order={4}>{t(($) => $.analytics.sections.automations)}</Title>
              <SimpleGrid cols={2}>
                <StatCard
                  label={t(($) => $.analytics.automations.total)}
                  value={analyticsQuery.data.automations.total}
                />
                <StatCard
                  label={t(($) => $.analytics.automations.active)}
                  value={analyticsQuery.data.automations.active}
                />
                <StatCard
                  label={t(($) => $.analytics.automations.runsActive)}
                  value={analyticsQuery.data.automations.runsActive}
                />
                <StatCard
                  label={t(($) => $.analytics.automations.runsCompleted)}
                  value={analyticsQuery.data.automations.runsCompleted}
                />
              </SimpleGrid>
            </Stack>
          </SimpleGrid>
        </Stack>
      )}
    </Stack>
  )
}
