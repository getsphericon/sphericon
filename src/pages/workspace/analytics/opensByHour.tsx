import { EmptyState, Stack, Table, Text } from '@mantine/core'
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

const HOURS = Array.from({ length: 24 }, (_, hour) => hour)

// shade is the blue of a cell: the busiest hour is the darkest, an empty one is plain gray.
function shade(opens: number, busiest: number) {
  if (opens === 0) {
    return 'gray.0'
  }
  return `blue.${Math.min(9, 1 + Math.ceil((opens / busiest) * 8))}`
}

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

  const opens = new Map(query.data.cells.map((c) => [`${c.weekday}-${c.hour}`, c.opens]))
  const busiest = Math.max(...query.data.cells.map((c) => c.opens))

  return (
    <Stack gap="xs">
      <Text size="sm" c="dimmed">
        {t(($) => $.analytics.opensByHour.note)}
      </Text>
      <Table.ScrollContainer minWidth={720}>
        <Table withColumnBorders>
          <Table.Thead>
            <Table.Tr>
              <Table.Th />
              {HOURS.map((hour) => (
                <Table.Th key={hour} ta="center" px={2}>
                  {hour}
                </Table.Th>
              ))}
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {WEEKDAYS.map((key, index) => {
              const day = t(($) => $.analytics.opensByHour.weekdays[key])
              return (
                <Table.Tr key={key}>
                  <Table.Th>{day}</Table.Th>
                  {HOURS.map((hour) => {
                    const count = opens.get(`${index + 1}-${hour}`) ?? 0
                    const label = t(($) => $.analytics.opensByHour.cell, {
                      day,
                      hour: String(hour).padStart(2, '0'),
                      count,
                    })
                    return (
                      <Table.Td
                        key={hour}
                        bg={shade(count, busiest)}
                        aria-label={label}
                        title={label}
                      />
                    )
                  })}
                </Table.Tr>
              )
            })}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>
    </Stack>
  )
}
