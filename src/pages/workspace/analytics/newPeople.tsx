import { BarChart } from '@mantine/charts'
import { EmptyState, SimpleGrid, Stack, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { StatCard } from '../../../components/StatCard.tsx'
import { siteAnalyticsNewPeopleOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import { useAnalyticsRange } from '../../../hooks/useAnalyticsRange.ts'
import { Route as newPeopleRoute } from '../../../routes/workspaces/$workspaceId/analytics/new-people.tsx'
import { ReportState } from './ReportState.tsx'

// NewPeoplePage is the New people report: the Contacts created per Report day, by Contact source.
export function NewPeoplePage() {
  const { t } = useTranslation()
  const { workspaceId } = newPeopleRoute.useParams()
  const range = useAnalyticsRange()

  const reportQuery = useQuery(
    siteAnalyticsNewPeopleOptions({ path: { workspaceId }, query: { range } }),
  )

  return (
    <Stack>
      <ReportState query={reportQuery} />

      {reportQuery.data && (
        <Stack gap="md">
          <Text size="sm" c="dimmed">
            {t(($) => $.analytics.newPeople.note)}
          </Text>
          <SimpleGrid cols={{ base: 2, sm: 4 }}>
            <StatCard
              label={t(($) => $.analytics.newPeople.total)}
              value={reportQuery.data.total}
            />
          </SimpleGrid>
          {reportQuery.data.total === 0 ? (
            <EmptyState title={t(($) => $.analytics.newPeople.empty)} />
          ) : (
            <BarChart
              h={300}
              data={reportQuery.data.timeseries}
              dataKey="date"
              type="stacked"
              withLegend
              series={[
                { name: 'manual', label: t(($) => $.analytics.sources.manual), color: 'blue.6' },
                { name: 'api', label: t(($) => $.analytics.sources.api), color: 'teal.6' },
                { name: 'tracker', label: t(($) => $.analytics.sources.tracker), color: 'grape.6' },
                { name: 'unknown', label: t(($) => $.analytics.sources.unknown), color: 'gray.5' },
              ]}
            />
          )}
        </Stack>
      )}
    </Stack>
  )
}
