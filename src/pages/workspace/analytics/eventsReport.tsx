import { EmptyState, Stack } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useTranslation } from 'react-i18next'

import { AnchorLink } from '../../../components/RouterLink.tsx'
import { siteAnalyticsEventsOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import { useAnalyticsRange } from '../../../hooks/useAnalyticsRange.ts'
import { Route as eventsRoute } from '../../../routes/workspaces/$workspaceId/analytics/events.tsx'
import { Route as recentActivityRoute } from '../../../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { formatDateTime } from '../../../utils/datetime.ts'
import { ReportState } from './ReportState.tsx'

// EventsReportPage is the Events report: every action of the section's window with how often it
// happened, for how many Contacts and when it last did. An action opens Recent activity on it.
export function EventsReportPage() {
  const { t } = useTranslation()
  const { workspaceId } = eventsRoute.useParams()
  const range = useAnalyticsRange()

  const eventsQuery = useQuery(
    siteAnalyticsEventsOptions({ path: { workspaceId }, query: { range } }),
  )
  const actions = eventsQuery.data?.actions ?? []

  return (
    <Stack>
      <ReportState query={eventsQuery} errorMessage={t(($) => $.analytics.events.loadError)} />

      {eventsQuery.data &&
        (actions.length === 0 ? (
          <EmptyState title={t(($) => $.analytics.events.empty)} />
        ) : (
          <DataTable
            withTableBorder
            highlightOnHover
            records={actions}
            idAccessor="action"
            columns={[
              {
                accessor: 'action',
                title: t(($) => $.analytics.events.action),
                render: (record) => (
                  <AnchorLink
                    to={recentActivityRoute.to}
                    params={{ workspaceId }}
                    search={{ range, action: record.action }}
                  >
                    {record.action}
                  </AnchorLink>
                ),
              },
              {
                accessor: 'count',
                title: t(($) => $.analytics.events.count),
                textAlign: 'right',
              },
              {
                accessor: 'contacts',
                title: t(($) => $.analytics.events.contacts),
                textAlign: 'right',
              },
              {
                accessor: 'lastAt',
                title: t(($) => $.analytics.events.lastAt),
                render: (record) => formatDateTime(record.lastAt),
              },
            ]}
          />
        ))}
    </Stack>
  )
}
