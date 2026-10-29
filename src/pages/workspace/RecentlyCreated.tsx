import { EmptyState, Stack } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { DataTable } from 'mantine-datatable'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteAutomationsListOptions,
  siteBroadcastsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as automationsDetailRoute } from '../../routes/workspaces/$workspaceId/automations/$automationId/index.tsx'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { formatDateTime } from '../../utils/datetime.ts'

const LIMIT = 5

interface Recent {
  kind: 'broadcast' | 'automation'
  id: string
  name: string
  status: string
  createdAt: string
}

// RecentlyCreated lists what the Workspace created last, broadcasts and automations together,
// newest first: each source is asked for its newest few and the merge keeps the newest overall.
export function RecentlyCreated({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const query = { page: 1, pageSize: LIMIT }
  const broadcasts = useQuery(siteBroadcastsListOptions({ path: { workspaceId }, query }))
  const automations = useQuery(siteAutomationsListOptions({ path: { workspaceId }, query }))

  const error = broadcasts.error ?? automations.error
  if (error) {
    const title = t(($) => $.overview.recentLoadError)
    return <ApiErrorAlert error={error} title={title} fallback={title} />
  }
  if (broadcasts.isLoading || automations.isLoading) {
    return <TableSkeleton />
  }

  const records: Recent[] = [
    ...(broadcasts.data?.items ?? []).map((b) => ({
      kind: 'broadcast' as const,
      id: b.id,
      name: b.name,
      status: t(($) => $.broadcasts.status[b.status]),
      createdAt: b.createdAt,
    })),
    ...(automations.data?.items ?? []).map((a) => ({
      kind: 'automation' as const,
      id: a.id,
      name: a.name,
      status:
        a.status === 'active'
          ? t(($) => $.automations.statusActive)
          : t(($) => $.automations.statusDraft),
      createdAt: a.createdAt,
    })),
  ]
    .toSorted((a, b) => Date.parse(b.createdAt) - Date.parse(a.createdAt))
    .slice(0, LIMIT)

  return (
    <Stack>
      <DataTable<Recent>
        minHeight={records.length > 0 ? 0 : 200}
        withTableBorder
        highlightOnHover
        idAccessor={(record) => `${record.kind}:${record.id}`}
        records={records}
        onRowClick={({ record }) =>
          void (record.kind === 'broadcast'
            ? navigate({
                to: broadcastsReportRoute.to,
                params: { workspaceId, broadcastId: record.id },
              })
            : navigate({
                to: automationsDetailRoute.to,
                params: { workspaceId, automationId: record.id },
              }))
        }
        columns={[
          { accessor: 'name', title: t(($) => $.overview.nameLabel) },
          {
            accessor: 'kind',
            title: t(($) => $.overview.typeLabel),
            render: (record) =>
              record.kind === 'broadcast'
                ? t(($) => $.overview.typeBroadcast)
                : t(($) => $.overview.typeAutomation),
          },
          {
            accessor: 'createdAt',
            title: t(($) => $.overview.createdLabel),
            render: (record) => formatDateTime(record.createdAt),
          },
          { accessor: 'status', title: t(($) => $.overview.statusLabel) },
        ]}
        noRecordsText={t(($) => $.overview.recentEmpty)}
        emptyState={<EmptyState title={t(($) => $.overview.recentEmpty)} />}
      />
    </Stack>
  )
}
