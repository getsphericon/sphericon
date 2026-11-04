import { useSuspenseQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useTranslation } from 'react-i18next'

import { CenteredEmptyState } from '../../../components/CenteredEmptyState.tsx'
import { EventActionName } from '../../../components/EventActionName.tsx'
import { RelativeTime } from '../../../components/RelativeTime.tsx'
import { AnchorLink } from '../../../components/RouterLink.tsx'
import { siteAnalyticsEventsOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import { useAnalyticsRange } from '../../../hooks/useAnalyticsRange.ts'
import { Route as activityRoute } from '../../../routes/workspaces/$workspaceId/activity.tsx'
import { Route as eventsRoute } from '../../../routes/workspaces/$workspaceId/analytics/events.tsx'
import { ReportBoundary } from './ReportBoundary.tsx'

// EventsReportPage is the Events report: every action of the section's window with how often it
// happened, for how many Contacts and when it last did. An action opens the Activity feed on it.
export function EventsReportPage() {
  const { t } = useTranslation()
  return (
    <ReportBoundary errorTitle={t(($) => $.analytics.events.loadError)}>
      <EventsReport />
    </ReportBoundary>
  )
}

function EventsReport() {
  const { t } = useTranslation()
  const { workspaceId } = eventsRoute.useParams()
  const range = useAnalyticsRange()

  const actions = useSuspenseQuery(
    siteAnalyticsEventsOptions({ path: { workspaceId }, query: { range } }),
  ).data.actions

  return actions.length === 0 ? (
    <CenteredEmptyState title={t(($) => $.analytics.events.empty)} />
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
              to={activityRoute.to}
              params={{ workspaceId }}
              search={{ action: record.action }}
            >
              <EventActionName action={record.action} />
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
          render: (record) => <RelativeTime value={record.lastAt} />,
        },
      ]}
    />
  )
}
