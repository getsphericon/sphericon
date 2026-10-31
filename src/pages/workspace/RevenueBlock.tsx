import { Alert, Card, Group, SegmentedControl, SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import { StatCard } from '../../components/StatCard.tsx'
import { siteAnalyticsRevenueOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { SiteAnalyticsRange } from '../../generated/site/types.gen.ts'
import { Route as settingsRoute } from '../../routes/workspaces/$workspaceId/settings/index.tsx'
import { formatMoney } from '../../utils/money.ts'
import { formatPercent } from '../../utils/percent.ts'

const PERIODS = [SiteAnalyticsRange['7D'], SiteAnalyticsRange['30D'], SiteAnalyticsRange['90D']]

// RevenueFigures is the period's revenue, or the call to choose the Reporting currency; the block's
// boundary shows the load and its failure.
function RevenueFigures({
  workspaceId,
  range,
}: {
  workspaceId: string
  range: SiteAnalyticsRange
}) {
  const { t } = useTranslation()
  const revenue = useSuspenseQuery(
    siteAnalyticsRevenueOptions({ path: { workspaceId }, query: { range } }),
  ).data
  const currency = revenue.reportingCurrency

  return currency === null ? (
    <Alert color="blue" title={t(($) => $.overview.revenue.currencyUnsetTitle)}>
      <Stack gap="xs" align="flex-start">
        <Text size="sm">{t(($) => $.overview.revenue.currencyUnsetBody)}</Text>
        <ButtonLink to={settingsRoute.to} params={{ workspaceId }} variant="default" size="xs">
          {t(($) => $.overview.revenue.chooseCurrency)}
        </ButtonLink>
      </Stack>
    </Alert>
  ) : (
    <SimpleGrid cols={{ base: 1, sm: 3 }}>
      <StatCard
        label={t(($) => $.overview.revenue.total)}
        value={formatMoney(revenue.total, currency)}
      />
      <StatCard
        label={t(($) => $.overview.revenue.credited)}
        value={revenue.total > 0 ? formatPercent(revenue.credited / revenue.total) : '–'}
      />
      <StatCard label={t(($) => $.overview.revenue.orders)} value={revenue.orders} />
    </SimpleGrid>
  )
}

// RevenueBlock is the overview's revenue of the orders sent to Sphericon for a period. Until the
// Reporting currency is chosen it shows the call to choose it and no figure.
export function RevenueBlock({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const [range, setRange] = useState<SiteAnalyticsRange>(SiteAnalyticsRange['30D'])

  return (
    <Card withBorder padding="lg">
      <Stack gap="sm">
        <Group justify="space-between">
          <Title order={4}>{t(($) => $.overview.revenue.title)}</Title>
          <SegmentedControl
            size="xs"
            value={range}
            onChange={(value) => {
              const next = PERIODS.find((period) => period === value)
              if (next) {
                setRange(next)
              }
            }}
            data={PERIODS.map((period) => ({
              value: period,
              label: t(($) => $.overview.revenue.periods[period]),
            }))}
          />
        </Group>

        {/* A new period starts over, so a failed one does not stay on screen. */}
        <QueryBoundary
          key={range}
          fallback={<BlockSkeleton />}
          errorTitle={t(($) => $.overview.revenue.loadError)}
        >
          <RevenueFigures workspaceId={workspaceId} range={range} />
        </QueryBoundary>

        <Text size="xs" c="dimmed">
          {t(($) => $.overview.revenue.note)}
        </Text>
      </Stack>
    </Card>
  )
}
