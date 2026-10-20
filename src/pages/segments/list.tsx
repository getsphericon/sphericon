import { Button, Group, Stack, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteSegmentsDeleteMutation,
  siteSegmentsListOptions,
  siteSegmentsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { Route as segmentsEditRoute } from '../../routes/workspaces/$workspaceId/segments/$segmentId/edit.tsx'
import { Route as segmentsRoute } from '../../routes/workspaces/$workspaceId/segments/index.tsx'
import { Route as segmentsCreateRoute } from '../../routes/workspaces/$workspaceId/segments/new.tsx'

export function SegmentsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = segmentsRoute.useParams()

  const deleteSegment = useDeleteResource({
    mutation: siteSegmentsDeleteMutation(),
    workspaceId,
    listQueryKey: siteSegmentsListQueryKey({ path: { workspaceId } }),
    successMessage: t(($) => $.notifications.segmentDeleted),
    errorTitle: t(($) => $.alerts.segmentDeleteErrorTitle),
  })

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.segments)}</Title>
        <ButtonLink to={segmentsCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.segments.addSegment)}
        </ButtonLink>
      </Group>

      <PagedTable
        options={(query) => siteSegmentsListOptions({ path: { workspaceId }, query })}
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
                <Button
                  size="compact-sm"
                  color="red"
                  variant="light"
                  onClick={() => deleteSegment(record.id)}
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
