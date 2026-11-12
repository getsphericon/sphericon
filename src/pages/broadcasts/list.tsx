import { Group, Menu, Select, Stack, Title } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ArchiveViewControl } from '../../components/ArchiveViewControl.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink, MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteBroadcastsArchiveMutation,
  siteBroadcastsListOptions,
  siteBroadcastsUnarchiveMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteBroadcastResource } from '../../generated/site/types.gen.ts'
import { zBroadcastChannel } from '../../generated/site/zod.gen.ts'
import { useFormat } from '../../hooks/useFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/edit.tsx'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$workspaceId/broadcasts/$broadcastId/report.tsx'
import { Route as broadcastsRoute } from '../../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$workspaceId/broadcasts/new.tsx'
import { withPage } from '../../searchSchemas.ts'
import { messageRevenueCells } from '../../utils/messageRevenue.ts'
import type { FieldColumn } from '../../utils/tableColumns.ts'
import { BroadcastHoldBadge } from './BroadcastHold.tsx'
import { BroadcastStatusBadge } from './BroadcastStatusBadge.tsx'

export function BroadcastsListPage() {
  const { t } = useTranslation()
  const { locale } = useFormat()
  const { workspaceId } = broadcastsRoute.useParams()
  const navigate = useNavigate()

  const archiveBroadcast = useResourceMutation({
    mutation: siteBroadcastsArchiveMutation(),
    successMessage: t(($) => $.notifications.broadcastArchived),
    errorTitle: t(($) => $.alerts.broadcastArchiveErrorTitle),
  })
  const restoreBroadcast = useResourceMutation({
    mutation: siteBroadcastsUnarchiveMutation(),
    successMessage: t(($) => $.notifications.broadcastRestored),
    errorTitle: t(($) => $.alerts.broadcastRestoreErrorTitle),
  })
  const { archived = false, channel, page } = broadcastsRoute.useSearch()
  const routeNavigate = broadcastsRoute.useNavigate()

  return (
    <Stack flex={1}>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.broadcasts)}</Title>
        <ButtonLink to={broadcastsCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.broadcasts.addBroadcast)}
        </ButtonLink>
      </Group>

      <Group align="flex-end">
        <ArchiveViewControl
          archived={archived}
          onChange={(next) =>
            void routeNavigate({
              search: { archived: next || undefined, ...(channel && { channel }) },
              replace: true,
              viewTransition: false,
            })
          }
        />
        <Select
          w={200}
          clearable
          label={t(($) => $.broadcasts.channelFilter)}
          placeholder={t(($) => $.broadcasts.channelAll)}
          data={zBroadcastChannel.options.map((value) => ({
            value,
            label: t(($) => $.broadcasts.channels[value]),
          }))}
          value={channel ?? null}
          onChange={(next) =>
            void routeNavigate({
              // A new filter starts again from the first page.
              search: {
                archived: archived || undefined,
                channel: zBroadcastChannel
                  .optional()
                  .catch(undefined)
                  .parse(next ?? undefined),
              },
              replace: true,
              viewTransition: false,
            })
          }
        />
      </Group>

      <PagedTable
        page={page}
        onPageChange={(next) =>
          void routeNavigate({ search: withPage(next), viewTransition: false })
        }
        options={(query) =>
          siteBroadcastsListOptions({
            path: { workspaceId },
            query: { ...query, archived, ...(channel && { channel }) },
          })
        }
        loadErrorTitle={t(($) => $.alerts.broadcastLoadErrorTitle)}
        collection={t(($) => $.nav.broadcasts)}
        empty={{
          title: t(($) => $.broadcasts.noRecords),
          action: (
            <ButtonLink variant="default" to={broadcastsCreateRoute.to} params={{ workspaceId }}>
              {t(($) => $.broadcasts.addBroadcast)}
            </ButtonLink>
          ),
        }}
        onRowClick={(record) =>
          void navigate({
            to: broadcastsReportRoute.to,
            params: { workspaceId, broadcastId: record.id },
          })
        }
        columns={[
          {
            accessor: 'name',
            title: t(($) => $.broadcasts.nameLabel),
          } satisfies FieldColumn<SiteBroadcastResource>,
          {
            accessor: 'channel',
            title: t(($) => $.broadcasts.channelLabel),
            render: (record) => t(($) => $.broadcasts.channels[record.channel]),
          },
          {
            accessor: 'status',
            title: t(($) => $.broadcasts.statusLabel),
            render: (record) => (
              <Group gap="xs">
                <BroadcastStatusBadge status={record.status} />
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
            render: (record) => record.stats.openedCount ?? '–',
          },
          {
            accessor: 'orders',
            title: t(($) => $.messageRevenue.orders),
            render: (record) => messageRevenueCells(record.stats.revenue, locale).orders,
          },
          {
            accessor: 'revenue',
            title: t(($) => $.messageRevenue.revenue),
            render: (record) => messageRevenueCells(record.stats.revenue, locale).revenue,
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
                    disabled={record.status !== 'draft' || Boolean(record.archivedAt)}
                    to={broadcastsEditRoute.to}
                    params={{ workspaceId, broadcastId: record.id }}
                  >
                    {t(($) => $.actions.edit)}
                  </MenuItemLink>
                  {record.status !== 'scheduled' &&
                    record.status !== 'sending' &&
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
