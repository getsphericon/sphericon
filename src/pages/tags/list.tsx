import { Group, Menu, Stack, Title } from '@mantine/core'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteTagsDeleteMutation,
  siteTagsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { Route as tagsRoute } from '../../routes/workspaces/$workspaceId/tags/index.tsx'
import { withPage } from '../../searchSchemas.ts'

export function TagsListPage() {
  const { formatDateTime } = useDateFormat()
  const { page } = tagsRoute.useSearch()
  const pageNavigate = tagsRoute.useNavigate()
  const { t } = useTranslation()
  const { workspaceId } = tagsRoute.useParams()
  const navigate = useNavigate()

  const deleteTag = useDeleteResource({
    mutation: siteTagsDeleteMutation(),
    workspaceId,
    successMessage: t(($) => $.notifications.tagDeleted),
    errorTitle: t(($) => $.alerts.tagDeleteErrorTitle),
  })

  return (
    <Stack>
      <Title order={2}>{t(($) => $.nav.tags)}</Title>

      <PagedTable
        page={page}
        onPageChange={(next) =>
          void pageNavigate({ search: withPage(next), viewTransition: false })
        }
        options={(query) => siteTagsListOptions({ path: { workspaceId }, query })}
        loadErrorTitle={t(($) => $.alerts.tagLoadErrorTitle)}
        noRecordsText={t(($) => $.tags.noRecords)}
        emptyState={t(($) => $.tags.emptyState)}
        // A Tag opens the list of the contacts that carry it.
        onRowClick={(record) =>
          void navigate({
            to: contactsRoute.to,
            params: { workspaceId },
            search: { tag: record.name },
          })
        }
        columns={[
          { accessor: 'name', title: t(($) => $.tags.nameLabel) },
          {
            accessor: 'createdAt',
            title: t(($) => $.tags.createdAtLabel),
            render: (record) => formatDateTime(record.createdAt),
          },
          {
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Group justify="flex-end">
                <RowActionsMenu>
                  <MenuItemLink
                    to={contactsRoute.to}
                    params={{ workspaceId }}
                    search={{ tag: record.name }}
                  >
                    {t(($) => $.tags.viewContacts)}
                  </MenuItemLink>
                  <Menu.Item color="red" onClick={() => deleteTag(record.id)}>
                    {t(($) => $.actions.delete)}
                  </Menu.Item>
                </RowActionsMenu>
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
