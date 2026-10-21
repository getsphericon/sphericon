import { Button, Group, Stack, Switch, Title } from '@mantine/core'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteSegmentsArchiveMutation,
  siteSegmentsListOptions,
  siteSegmentsListQueryKey,
  siteSegmentsUnarchiveMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as segmentsEditRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/edit.tsx'
import { Route as segmentsRoute } from '../../routes/workspaces/$workspaceId/segments/index.tsx'
import { Route as segmentsCreateRoute } from '../../routes/workspaces/$workspaceId/segments/new.tsx'

export function SegmentsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = segmentsRoute.useParams()

  const archiveSegment = useResourceMutation({
    mutation: siteSegmentsArchiveMutation(),
    invalidate: [siteSegmentsListQueryKey({ path: { workspaceId } })],
    successMessage: t(($) => $.notifications.segmentArchived),
    errorTitle: t(($) => $.alerts.segmentArchiveErrorTitle),
  })
  const restoreSegment = useResourceMutation({
    mutation: siteSegmentsUnarchiveMutation(),
    invalidate: [siteSegmentsListQueryKey({ path: { workspaceId } })],
    successMessage: t(($) => $.notifications.segmentRestored),
    errorTitle: t(($) => $.alerts.segmentRestoreErrorTitle),
  })
  const [archived, setArchived] = useState(false)

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.segments)}</Title>
        <ButtonLink to={segmentsCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.segments.addSegment)}
        </ButtonLink>
      </Group>

      <Switch
        label={t(($) => $.segments.showArchived)}
        checked={archived}
        onChange={(event) => setArchived(event.currentTarget.checked)}
      />

      <PagedTable
        key={String(archived)}
        options={(query) =>
          siteSegmentsListOptions({ path: { workspaceId }, query: { ...query, archived } })
        }
        loadErrorTitle={t(($) => $.alerts.segmentLoadErrorTitle)}
        noRecordsText={t(($) => $.segments.noRecords)}
        emptyState={t(($) => $.segments.emptyState)}
        columns={[
          { accessor: 'name', title: t(($) => $.segments.nameLabel) },
          {
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Group gap="xs">
                <ButtonLink
                  size="compact-sm"
                  to={segmentsEditRoute.to}
                  params={{ workspaceId, segmentId: record.id }}
                >
                  {t(($) => $.actions.edit)}
                </ButtonLink>
                {record.archivedAt ? (
                  <Button
                    size="compact-sm"
                    variant="light"
                    onClick={() => restoreSegment.mutate({ path: { workspaceId, id: record.id } })}
                  >
                    {t(($) => $.actions.unarchive)}
                  </Button>
                ) : (
                  <Button
                    size="compact-sm"
                    color="red"
                    variant="light"
                    onClick={() => archiveSegment.mutate({ path: { workspaceId, id: record.id } })}
                  >
                    {t(($) => $.actions.archive)}
                  </Button>
                )}
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
