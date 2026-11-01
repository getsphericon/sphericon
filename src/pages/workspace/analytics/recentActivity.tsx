import { EmptyState, Badge, Group, Stack, Select, Switch, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../../components/ApiErrorAlert.tsx'
import { EventActionName, EventActionOption } from '../../../components/EventActionName.tsx'
import { DetailedEventRowDetails } from '../../../components/EventRowDetails.tsx'
import { RelativeTime } from '../../../components/RelativeTime.tsx'
import { TableSkeleton } from '../../../components/Skeletons.tsx'
import {
  siteEventsActionsOptions,
  siteEventsListOptions,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import { useEventActionFilter, useEventActionLabel } from '../../../hooks/useEventActionLabel.ts'
import { Route as recentActivityRoute } from '../../../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { Route as workspaceRoute } from '../../../routes/workspaces/$workspaceId/route.tsx'
import { withPage } from '../../../searchSchemas.ts'

const PAGE_SIZE = 25
const LIVE_REFETCH_MS = 5000

// RecentActivityPage is a read-only feed of the workspace's tracking events, most
// recent first. Live mode polls so newly tracked events surface without a
// manual reload — the primary way to verify that tracking works.
export function RecentActivityPage() {
  const { t } = useTranslation()
  const actionFilter = useEventActionFilter()
  const { workspaceId } = workspaceRoute.useParams()
  const [live, setLive] = useState(true)
  const labelOf = useEventActionLabel()
  // The filter and the page are the URL's; another filter starts on its first page.
  const { action = '', page = 1 } = recentActivityRoute.useSearch()
  const navigate = recentActivityRoute.useNavigate()
  const applyAction = (next: string | null) =>
    void navigate({
      search: { action: next ?? undefined },
      replace: true,
      viewTransition: false,
    })
  // The actions the Workspace has recorded, a System event by its name; the URL's own stays
  // shown while the list loads or when it names an action not recorded.
  const actionsQuery = useQuery(siteEventsActionsOptions({ path: { workspaceId } }))
  const actionNames = actionsQuery.data?.actions ?? []
  const actionOptions = (
    action && !actionNames.includes(action) ? [action, ...actionNames] : actionNames
  ).map((name) => ({ value: name, label: labelOf(name) }))
  const setPage = (next: number) =>
    void navigate({
      search: withPage(next),
      viewTransition: false,
    })

  const eventsQuery = useQuery({
    ...siteEventsListOptions({
      path: { workspaceId },
      query: {
        page,
        pageSize: PAGE_SIZE,
        ...(action ? { action } : {}),
      },
    }),
    refetchInterval: live ? LIVE_REFETCH_MS : false,
  })

  const records = eventsQuery.data?.items ?? []
  const totalItems = eventsQuery.data?.totalItems ?? 0

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.activity.title)}</Title>
        <Group gap="md">
          <Select
            placeholder={t(($) => $.activity.filterActionPlaceholder)}
            searchable
            clearable
            data={actionOptions}
            renderOption={({ option }) => <EventActionOption action={option.value} />}
            filter={actionFilter}
            value={action || null}
            onChange={applyAction}
          />
          <Switch
            label={t(($) => $.activity.live)}
            checked={live}
            onChange={(event) => setLive(event.currentTarget.checked)}
          />
        </Group>
      </Group>

      {eventsQuery.isError ? (
        <ApiErrorAlert error={eventsQuery.error} title={t(($) => $.activity.loadErrorTitle)} />
      ) : null}

      {eventsQuery.isLoading ? <TableSkeleton /> : null}

      <DataTable
        minHeight={records.length === 0 ? 200 : 0}
        withTableBorder
        highlightOnHover
        records={records}
        idAccessor="id"
        columns={[
          {
            accessor: 'createdAt',
            title: t(($) => $.activity.time),
            render: (record) => <RelativeTime value={record.createdAt} />,
          },
          {
            accessor: 'action',
            title: t(($) => $.activity.action),
            render: (record) => (
              <Group gap="xs">
                <EventActionName action={record.action} />
                {record.properties?.captured === true ? (
                  <Badge color="gray" variant="outline">
                    {t(($) => $.activity.captured)}
                  </Badge>
                ) : null}
              </Group>
            ),
          },
          {
            accessor: 'subject',
            title: t(($) => $.activity.subject),
            render: (record) => record.email ?? record.subjectId,
          },
        ]}
        rowExpansion={{ content: DetailedEventRowDetails }}
        totalRecords={totalItems}
        recordsPerPage={PAGE_SIZE}
        page={page}
        onPageChange={setPage}
        fetching={eventsQuery.isFetching}
        emptyState={<EmptyState title={t(($) => $.activity.noRecords)} />}
      />
    </Stack>
  )
}
