import { BarChart } from '@mantine/charts'
import { EmptyState, SimpleGrid, Stack, Text } from '@mantine/core'
import { IconChartBar } from '@tabler/icons-react'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { StatCard } from '../../../components/StatCard.tsx'
import { siteAnalyticsNewPeopleOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import { useAnalyticsRange } from '../../../hooks/useAnalyticsRange.ts'
import { Route as newPeopleRoute } from '../../../routes/workspaces/$workspaceId/analytics/new-people.tsx'
import { ReportBoundary } from './ReportBoundary.tsx'

// NewPeoplePage is the New people report: the Contacts created per Report day, by Contact source.
export function NewPeoplePage() {
  return (
    <ReportBoundary>
      <NewPeopleReport />
    </ReportBoundary>
  )
}

function NewPeopleReport() {
  const { t } = useTranslation()
  const { workspaceId } = newPeopleRoute.useParams()
  const range = useAnalyticsRange()

  const report = useSuspenseQuery(
    siteAnalyticsNewPeopleOptions({ path: { workspaceId }, query: { range } }),
  ).data

  return (
    <Stack gap="md">
      <Text size="sm" c="dimmed">
        {t(($) => $.analytics.newPeople.note)}
      </Text>
      <SimpleGrid cols={{ base: 2, sm: 4 }}>
        <StatCard label={t(($) => $.analytics.newPeople.total)} value={report.total} />
      </SimpleGrid>
      {report.total === 0 ? (
        <EmptyState icon={<IconChartBar />} title={t(($) => $.analytics.newPeople.empty)} />
      ) : (
        <BarChart
          h={300}
          data={report.timeseries}
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
  )
}
