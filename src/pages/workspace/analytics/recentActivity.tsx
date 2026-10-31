import {
  EmptyState,
  Alert,
  Badge,
  Group,
  Stack,
  Switch,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useDebouncedCallback } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DetailedEventRowDetails } from '../../../components/EventRowDetails.tsx'
import { RelativeTime } from '../../../components/RelativeTime.tsx'
import { TableSkeleton } from '../../../components/Skeletons.tsx'
import { siteEventsListOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
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
  const { workspaceId } = workspaceRoute.useParams()
  const [live, setLive] = useState(true)
  // The filter and the page are the URL's; the field holds what is being typed.
  const { action = '', page = 1 } = recentActivityRoute.useSearch()
  const navigate = recentActivityRoute.useNavigate()
  const [actionFilter, setActionFilter] = useState(action)
  // A filter reaches the URL once typing pauses; another filter starts on its first page.
  const applyAction = useDebouncedCallback(
    (next: string) =>
      void navigate({
        search: { action: next.trim() || undefined },
        replace: true,
        viewTransition: false,
      }),
    300,
  )
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
          <TextInput
            placeholder={t(($) => $.activity.filterActionPlaceholder)}
            value={actionFilter}
            onChange={(event) => {
              setActionFilter(event.currentTarget.value)
              applyAction(event.currentTarget.value)
            }}
          />
          <Switch
            label={t(($) => $.activity.live)}
            checked={live}
            onChange={(event) => setLive(event.currentTarget.checked)}
          />
        </Group>
      </Group>

      {eventsQuery.isError ? (
        <Alert color="red" title={t(($) => $.activity.loadErrorTitle)}>
          {t(($) => $.activity.loadErrorTitle)}
        </Alert>
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
                <Text span>{record.action}</Text>
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
