import { Badge, Button, Group, Stack } from '@mantine/core'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteBroadcastsDeleteMutation,
  siteBroadcastsListOptions,
  siteBroadcastsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteBroadcastStatus } from '../../generated/site/types.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { Route as broadcastsEditRoute } from '../../routes/workspaces/$slug/broadcasts/$broadcastId/edit.tsx'
import { Route as broadcastsReportRoute } from '../../routes/workspaces/$slug/broadcasts/$broadcastId/report.tsx'
import { Route as broadcastsRoute } from '../../routes/workspaces/$slug/broadcasts/index.tsx'
import { Route as broadcastsCreateRoute } from '../../routes/workspaces/$slug/broadcasts/new.tsx'
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
  const { slug } = broadcastsRoute.useParams()

  const deleteBroadcast = useDeleteResource({
    mutation: siteBroadcastsDeleteMutation(),
    slug,
    listQueryKey: siteBroadcastsListQueryKey({ path: { slug } }),
    successMessage: t(($) => $.notifications.broadcastDeleted),
    errorTitle: t(($) => $.alerts.broadcastDeleteErrorTitle),
  })

  return (
    <Stack>
      <Group justify="flex-end">
        <ButtonLink to={broadcastsCreateRoute.to} params={{ slug }}>
          {t(($) => $.broadcasts.addBroadcast)}
        </ButtonLink>
      </Group>

      <PagedTable
        options={(query) => siteBroadcastsListOptions({ path: { slug }, query })}
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
                  params={{ slug, broadcastId: record.id }}
                >
                  {t(($) => $.broadcasts.report)}
                </ButtonLink>
                <ButtonLink
                  size="compact-sm"
                  disabled={record.status !== 'draft'}
                  to={broadcastsEditRoute.to}
                  params={{ slug, broadcastId: record.id }}
                >
                  {t(($) => $.actions.edit)}
                </ButtonLink>
                <Button
                  size="compact-sm"
                  color="red"
                  variant="light"
                  onClick={() => deleteBroadcast(record.id)}
                >
                  {t(($) => $.actions.delete)}
                </Button>
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
