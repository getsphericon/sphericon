import { BarChart } from '@mantine/charts'
import { EmptyState, SimpleGrid, Stack, Table, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../../components/ApiErrorAlert.tsx'
import { BlockSkeleton } from '../../../components/Skeletons.tsx'
import { StatCard } from '../../../components/StatCard.tsx'
import { siteAnalyticsSubscribedPeopleOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import { useAnalyticsRange } from '../../../hooks/useAnalyticsRange.ts'
import { Route as subscribedPeopleRoute } from '../../../routes/workspaces/$workspaceId/analytics/subscribed-people.tsx'

// signed is a net with its sign spelled out, so a day that lost people reads at a glance.
function signed(net: number) {
  return net > 0 ? `+${net}` : `${net}`
}

// SubscribedPeoplePage is the Subscribed people report: the current number beside the daily flow.
export function SubscribedPeoplePage() {
  const { t } = useTranslation()
  const { workspaceId } = subscribedPeopleRoute.useParams()
  const range = useAnalyticsRange()

  const reportQuery = useQuery(
    siteAnalyticsSubscribedPeopleOptions({ path: { workspaceId }, query: { range } }),
  )
  const report = reportQuery.data
  const changedDays = report?.series.filter((p) => p.subscribed > 0 || p.unsubscribed > 0) ?? []

  return (
    <Stack>
      {reportQuery.isLoading && <BlockSkeleton />}

      {reportQuery.isError && (
        <ApiErrorAlert
          error={reportQuery.error}
          title={t(($) => $.analytics.loadError)}
          fallback={t(($) => $.analytics.loadError)}
        />
      )}

      {report && (
        <Stack gap="xl">
          <SimpleGrid cols={{ base: 1, sm: 3 }}>
            <StatCard
              label={t(($) => $.analytics.subscribedPeople.current)}
              value={report.current}
            />
          </SimpleGrid>

          <Stack gap="xs">
            <Title order={4}>{t(($) => $.analytics.subscribedPeople.flow)}</Title>
            {changedDays.length === 0 ? (
              <EmptyState title={t(($) => $.analytics.subscribedPeople.empty)} />
            ) : (
              <>
                <BarChart
                  h={260}
                  data={report.series}
                  dataKey="date"
                  withLegend
                  series={[
                    {
                      name: 'subscribed',
                      label: t(($) => $.analytics.subscribedPeople.subscribed),
                      color: 'teal.6',
                    },
                    {
                      name: 'unsubscribed',
                      label: t(($) => $.analytics.subscribedPeople.unsubscribed),
                      color: 'gray.5',
                    },
                  ]}
                />
                <Table>
                  <Table.Thead>
                    <Table.Tr>
                      <Table.Th>{t(($) => $.analytics.subscribedPeople.day)}</Table.Th>
                      <Table.Th>{t(($) => $.analytics.subscribedPeople.subscribed)}</Table.Th>
                      <Table.Th>{t(($) => $.analytics.subscribedPeople.unsubscribed)}</Table.Th>
                      <Table.Th>{t(($) => $.analytics.subscribedPeople.net)}</Table.Th>
                    </Table.Tr>
                  </Table.Thead>
                  <Table.Tbody>
                    {changedDays.toReversed().map((p) => (
                      <Table.Tr key={p.date}>
                        <Table.Td>{p.date}</Table.Td>
                        <Table.Td>{p.subscribed}</Table.Td>
                        <Table.Td>{p.unsubscribed}</Table.Td>
                        <Table.Td>
                          <Text c={p.net < 0 ? 'red' : 'inherit'}>{signed(p.net)}</Text>
                        </Table.Td>
                      </Table.Tr>
                    ))}
                  </Table.Tbody>
                </Table>
              </>
            )}
          </Stack>
        </Stack>
      )}
    </Stack>
  )
}
