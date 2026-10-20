import { Button, Stack, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import {
  siteTagsDeleteMutation,
  siteTagsListOptions,
  siteTagsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { Route as tagsRoute } from '../../routes/workspaces/$workspaceId/tags/index.tsx'

export function TagsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = tagsRoute.useParams()

  const deleteTag = useDeleteResource({
    mutation: siteTagsDeleteMutation(),
    workspaceId,
    listQueryKey: siteTagsListQueryKey({ path: { workspaceId } }),
    successMessage: t(($) => $.notifications.tagDeleted),
    errorTitle: t(($) => $.alerts.tagDeleteErrorTitle),
  })

  return (
    <Stack>
      <Title order={2}>{t(($) => $.nav.tags)}</Title>

      <PagedTable
        options={(query) => siteTagsListOptions({ path: { workspaceId }, query })}
        loadErrorTitle={t(($) => $.alerts.tagLoadErrorTitle)}
        noRecordsText={t(($) => $.tags.noRecords)}
        emptyState={t(($) => $.tags.emptyState)}
        columns={[
          { accessor: 'name', title: t(($) => $.tags.nameLabel) },
          {
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Button
                size="compact-sm"
                color="red"
                variant="light"
                onClick={() => deleteTag(record.id)}
              >
                {t(($) => $.actions.delete)}
              </Button>
            ),
          },
        ]}
      />
    </Stack>
  )
}
