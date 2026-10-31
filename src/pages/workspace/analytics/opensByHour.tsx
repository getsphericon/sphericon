import { MatrixChart } from '@mantine/charts'
import { EmptyState, ScrollArea, Stack, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteAnalyticsOpensByHourOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import { useAnalyticsRange } from '../../../hooks/useAnalyticsRange.ts'
import { Route as opensByHourRoute } from '../../../routes/workspaces/$workspaceId/analytics/opens-by-hour.tsx'
import { ReportState } from './ReportState.tsx'

// ISO weekdays, Monday first: the report's `weekday` 1 to 7.
const WEEKDAYS = [
  'monday',
  'tuesday',
  'wednesday',
  'thursday',
  'friday',
  'saturday',
  'sunday',
] as const

const HOURS = Array.from({ length: 24 }, (_, hour) => String(hour))

// OpensByHourPage is the Opens by hour report: a weekday by hour grid of human opens, in the
// Workspace time zone, for the section's window.
export function OpensByHourPage() {
  const { t } = useTranslation()
  const { workspaceId } = opensByHourRoute.useParams()
  const range = useAnalyticsRange()

  const query = useQuery(
    siteAnalyticsOpensByHourOptions({ path: { workspaceId }, query: { range } }),
  )

  if (!query.data) {
    return <ReportState query={query} />
  }
  if (query.data.total === 0) {
    return <EmptyState title={t(($) => $.analytics.opensByHour.empty)} />
  }

  const days = WEEKDAYS.map((key) => t(($) => $.analytics.opensByHour.weekdays[key]))
  // A cell of the report as the chart takes it: the hour is the column, the weekday the row.
  const data = query.data.cells.map((cell) => ({
    x: String(cell.hour),
    y: days[cell.weekday - 1] ?? '',
    value: cell.opens,
  }))
  const cellLabel = (cell: { x: string; y: string; value: number | null }) =>
    t(($) => $.analytics.opensByHour.cell, {
      day: cell.y,
      hour: cell.x.padStart(2, '0'),
      count: cell.value ?? 0,
    })

  return (
    <Stack gap="xs">
      <Text size="sm" c="dimmed">
        {t(($) => $.analytics.opensByHour.note)}
      </Text>
      <ScrollArea>
        <MatrixChart
          aria-label={t(($) => $.analytics.nav.opensByHour)}
          data={data}
          xLabels={HOURS}
          yLabels={days}
          withXLabels
          withYLabels
          xLabelsPosition="top"
          xLabelsRotation={0}
          xLabelsHeight={24}
          yLabelsWidth={96}
          cellSize={26}
          colors={['blue.1', 'blue.3', 'blue.5', 'blue.7', 'blue.9']}
          withTooltip
          getTooltipLabel={cellLabel}
          getCellProps={(cell) => ({ 'aria-label': cellLabel(cell) })}
        />
      </ScrollArea>
    </Stack>
  )
}
