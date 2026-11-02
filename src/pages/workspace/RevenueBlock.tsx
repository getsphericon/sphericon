import { BarChart } from '@mantine/charts'
import { Alert, Card, SimpleGrid, Stack, Text, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { AnchorLink } from '../../components/RouterLink.tsx'
import { StatCard } from '../../components/StatCard.tsx'
import { siteAnalyticsRevenueOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteAnalyticsRange,
  SiteAnalyticsRevenuePeriod,
} from '../../generated/site/types.gen.ts'
import { Route as settingsRoute } from '../../routes/workspaces/$workspaceId/settings/index.tsx'
import { formatChange } from '../../utils/change.ts'
import { formatMoney } from '../../utils/money.ts'
import { formatPercent } from '../../utils/percent.ts'

// creditedShare is the part of a period's revenue credited to messages, or null when the period had
// no revenue to share out.
const creditedShare = (period: SiteAnalyticsRevenuePeriod) =>
  period.total > 0 ? period.credited / period.total : null

// formatShare is a credited share as a percentage, or a dash for a period with no revenue.
const formatShare = (share: number | null) => (share === null ? '–' : formatPercent(share))

// RevenueCard is the period's revenue once the Workspace has sent its first Order, and nothing
// before: a Workspace that sends no Orders never sees the block. A Reporting currency chosen before
// any Order does not make it appear. Orders in another currency are named, as they count in no sum.
// Each figure has the previous period under it, with the change since.
function RevenueCard({ workspaceId, range }: { workspaceId: string; range: SiteAnalyticsRange }) {
  const { t } = useTranslation()
  const revenue = useSuspenseQuery(
    siteAnalyticsRevenueOptions({ path: { workspaceId }, query: { range } }),
  ).data
  const currency = revenue.reportingCurrency
  if (revenue.firstOrderAt === null || currency === null) {
    return null
  }

  const { current, previous } = revenue
  const label = t(($) => $.overview.previous[range])
  const share = creditedShare(current)
  const shareBefore = creditedShare(previous)
  const shareChange =
    share === null || shareBefore === null ? null : formatChange(share, shareBefore)

  return (
    <Card withBorder padding="lg">
      <Stack gap="sm">
        <Title order={4}>{t(($) => $.overview.revenue.title)}</Title>
        <SimpleGrid cols={{ base: 1, sm: 3 }}>
          <StatCard
            label={t(($) => $.overview.revenue.total)}
            value={formatMoney(current.total, currency)}
            comparison={{
              label,
              value: formatMoney(previous.total, currency),
              change: formatChange(current.total, previous.total),
            }}
          />
          <StatCard
            label={t(($) => $.overview.revenue.credited)}
            value={formatShare(share)}
            comparison={{ label, value: formatShare(shareBefore), change: shareChange }}
          />
          <StatCard
            label={t(($) => $.overview.revenue.orders)}
            value={current.orders}
            comparison={{
              label,
              value: previous.orders,
              change: formatChange(current.orders, previous.orders),
            }}
          />
        </SimpleGrid>
        {current.orders > 0 && (
          <BarChart
            h={160}
            data={revenue.series}
            dataKey="date"
            valueFormatter={(amount) => formatMoney(amount, currency)}
            series={[
              { name: 'amount', label: t(($) => $.overview.revenue.total), color: 'teal.6' },
            ]}
          />
        )}
        {current.otherCurrencyOrders.map((other) => (
          <Alert key={other.currency} color="yellow">
            <Text size="sm">
              {t(($) => $.overview.revenue.otherCurrency, {
                count: other.orders,
                currency: other.currency,
                reportingCurrency: currency,
              })}{' '}
              <AnchorLink to={settingsRoute.to} params={{ workspaceId }}>
                {t(($) => $.overview.revenue.changeCurrency)}
              </AnchorLink>
            </Text>
          </Alert>
        ))}
        <Text size="xs" c="dimmed">
          {t(($) => $.overview.revenue.note)}
        </Text>
      </Stack>
    </Card>
  )
}

// RevenueBlock is the overview's revenue of the orders sent to Sphericon for the page's period. It
// shows no skeleton while it loads: most Workspaces send no Orders, and for them the block is not
// there at all.
export function RevenueBlock({
  workspaceId,
  range,
}: {
  workspaceId: string
  range: SiteAnalyticsRange
}) {
  const { t } = useTranslation()
  return (
    <QueryBoundary key={range} fallback={null} errorTitle={t(($) => $.overview.revenue.loadError)}>
      <RevenueCard workspaceId={workspaceId} range={range} />
    </QueryBoundary>
  )
}
