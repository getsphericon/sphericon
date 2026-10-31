import { Badge, Group, Menu, Stack, Title } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import { ArchiveViewControl } from '../../components/ArchiveViewControl.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink, MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteBroadcastsArchiveMutation,
  siteBroadcastsDeleteMutation,
  siteBroadcastsListOptions,
  siteBroadcastsListQueryKey,
  siteBroadcastsUnarchiveMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteBroadcastStatus } from '../../generated/site/types.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/edit.tsx'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { Route as broadcastsRoute } from '../../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$workspaceId/broadcasts/new.tsx'
import { withPage } from '../../searchSchemas.ts'
import { messageRevenueCells } from '../../utils/messageRevenue.ts'
import { BroadcastHoldBadge } from './BroadcastHold.tsx'

const STATUS_COLORS: Record<SiteBroadcastStatus, string> = {
  draft: 'gray',
  scheduled: 'blue',
  sending: 'yellow',
  sent: 'teal',
  failed: 'red',
}

function StatusBadge({ t, status }: { t: TFunction; status: SiteBroadcastStatus }) {
  return (
    <Badge color={STATUS_COLORS[status]} variant="light">
      {t(($) => $.broadcasts.status[status])}
    </Badge>
  )
}

export function BroadcastsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = broadcastsRoute.useParams()
  const navigate = useNavigate()

  const deleteBroadcast = useDeleteResource({
    mutation: siteBroadcastsDeleteMutation(),
    workspaceId,
    listQueryKey: siteBroadcastsListQueryKey({ path: { workspaceId } }),
    successMessage: t(($) => $.notifications.broadcastDeleted),
    errorTitle: t(($) => $.alerts.broadcastDeleteErrorTitle),
  })

  const archiveBroadcast = useResourceMutation({
    mutation: siteBroadcastsArchiveMutation(),
    invalidate: [siteBroadcastsListQueryKey({ path: { workspaceId } })],
    successMessage: t(($) => $.notifications.broadcastArchived),
    errorTitle: t(($) => $.alerts.broadcastArchiveErrorTitle),
  })
  const restoreBroadcast = useResourceMutation({
    mutation: siteBroadcastsUnarchiveMutation(),
    invalidate: [siteBroadcastsListQueryKey({ path: { workspaceId } })],
    successMessage: t(($) => $.notifications.broadcastRestored),
    errorTitle: t(($) => $.alerts.broadcastRestoreErrorTitle),
  })
  const { archived = false, page } = broadcastsRoute.useSearch()
  const routeNavigate = broadcastsRoute.useNavigate()

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.broadcasts)}</Title>
        <ButtonLink to={broadcastsCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.broadcasts.addBroadcast)}
        </ButtonLink>
      </Group>

      <ArchiveViewControl
        archived={archived}
        onChange={(next) =>
          void routeNavigate({
            search: { archived: next || undefined },
            replace: true,
            viewTransition: false,
          })
        }
      />

      <PagedTable
        page={page}
        onPageChange={(next) =>
          void routeNavigate({ search: withPage(next), viewTransition: false })
        }
        options={(query) =>
          siteBroadcastsListOptions({ path: { workspaceId }, query: { ...query, archived } })
        }
        loadErrorTitle={t(($) => $.alerts.broadcastLoadErrorTitle)}
        noRecordsText={t(($) => $.broadcasts.noRecords)}
        emptyState={t(($) => $.broadcasts.emptyState)}
        onRowClick={(record) =>
          void navigate({
            to: broadcastsReportRoute.to,
            params: { workspaceId, broadcastId: record.id },
          })
        }
        columns={[
          { accessor: 'name', title: t(($) => $.broadcasts.nameLabel) },
          {
            accessor: 'status',
            title: t(($) => $.broadcasts.statusLabel),
            render: (record) => (
              <Group gap="xs">
                <StatusBadge t={t} status={record.status} />
                {record.holdReason && <BroadcastHoldBadge />}
              </Group>
            ),
          },
          {
            accessor: 'recipients',
            title: t(($) => $.broadcasts.recipientsLabel),
            render: (record) => record.stats.recipientsTotal,
          },
          {
            accessor: 'opened',
            title: t(($) => $.broadcasts.openedLabel),
            render: (record) => record.stats.openedCount,
          },
          {
            accessor: 'orders',
            title: t(($) => $.messageRevenue.orders),
            render: (record) => messageRevenueCells(record.stats.revenue).orders,
          },
          {
            accessor: 'revenue',
            title: t(($) => $.messageRevenue.revenue),
            render: (record) => messageRevenueCells(record.stats.revenue).revenue,
          },
          {
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Group justify="flex-end">
                <RowActionsMenu>
                  <MenuItemLink
                    to={broadcastsReportRoute.to}
                    params={{ workspaceId, broadcastId: record.id }}
                  >
                    {t(($) => $.broadcasts.report)}
                  </MenuItemLink>
                  <MenuItemLink
                    disabled={record.status !== 'draft'}
                    to={broadcastsEditRoute.to}
                    params={{ workspaceId, broadcastId: record.id }}
                  >
                    {t(($) => $.actions.edit)}
                  </MenuItemLink>
                  {record.status === 'draft' && (
                    <Menu.Item color="red" onClick={() => deleteBroadcast(record.id)}>
                      {t(($) => $.actions.delete)}
                    </Menu.Item>
                  )}
                  {(record.status === 'sent' || record.status === 'failed') &&
                    (record.archivedAt ? (
                      <Menu.Item
                        onClick={() =>
                          restoreBroadcast.mutate({ path: { workspaceId, id: record.id } })
                        }
                      >
                        {t(($) => $.actions.unarchive)}
                      </Menu.Item>
                    ) : (
                      <Menu.Item
                        color="red"
                        onClick={() =>
                          archiveBroadcast.mutate({ path: { workspaceId, id: record.id } })
                        }
                      >
                        {t(($) => $.actions.archive)}
                      </Menu.Item>
                    ))}
                </RowActionsMenu>
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
