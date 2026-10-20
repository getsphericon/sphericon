import { Badge, Button, Group, Stack, Switch, Title } from '@mantine/core'
import type { TFunction } from 'i18next'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
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
  const [archived, setArchived] = useState(false)

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.broadcasts)}</Title>
        <ButtonLink to={broadcastsCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.broadcasts.addBroadcast)}
        </ButtonLink>
      </Group>

      <Switch
        label={t(($) => $.broadcasts.showArchived)}
        checked={archived}
        onChange={(event) => setArchived(event.currentTarget.checked)}
      />

      <PagedTable
        key={String(archived)}
        options={(query) =>
          siteBroadcastsListOptions({ path: { workspaceId }, query: { ...query, archived } })
        }
        loadErrorTitle={t(($) => $.alerts.broadcastLoadErrorTitle)}
        noRecordsText={t(($) => $.broadcasts.noRecords)}
        emptyState={t(($) => $.broadcasts.emptyState)}
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
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Group gap="xs">
                <ButtonLink
                  size="compact-sm"
                  variant="light"
                  to={broadcastsReportRoute.to}
                  params={{ workspaceId, broadcastId: record.id }}
                >
                  {t(($) => $.broadcasts.report)}
                </ButtonLink>
                <ButtonLink
                  size="compact-sm"
                  disabled={record.status !== 'draft'}
                  to={broadcastsEditRoute.to}
                  params={{ workspaceId, broadcastId: record.id }}
                >
                  {t(($) => $.actions.edit)}
                </ButtonLink>
                {record.status === 'draft' && (
                  <Button
                    size="compact-sm"
                    color="red"
                    variant="light"
                    onClick={() => deleteBroadcast(record.id)}
                  >
                    {t(($) => $.actions.delete)}
                  </Button>
                )}
                {(record.status === 'sent' || record.status === 'failed') &&
                  (record.archivedAt ? (
                    <Button
                      size="compact-sm"
                      variant="light"
                      onClick={() =>
                        restoreBroadcast.mutate({ path: { workspaceId, id: record.id } })
                      }
                    >
                      {t(($) => $.actions.unarchive)}
                    </Button>
                  ) : (
                    <Button
                      size="compact-sm"
                      color="red"
                      variant="light"
                      onClick={() =>
                        archiveBroadcast.mutate({ path: { workspaceId, id: record.id } })
                      }
                    >
                      {t(($) => $.actions.archive)}
                    </Button>
                  ))}
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
