import { ActionIcon, Group, Stack, Title, Tooltip } from '@mantine/core'
import { IconEye, IconPencil, IconTrash } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ActionIconLink, ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteContactsDeleteMutation,
  siteContactsListOptions,
  siteContactsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDeleteResource } from '../../hooks/useDeleteResource.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/edit.tsx'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { Route as contactsCreateRoute } from '../../routes/workspaces/$workspaceId/contacts/new.tsx'

export function ContactsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = contactsRoute.useParams()

  const deleteContact = useDeleteResource({
    mutation: siteContactsDeleteMutation(),
    workspaceId,
    listQueryKey: siteContactsListQueryKey({ path: { workspaceId } }),
    successMessage: t(($) => $.notifications.contactDeleted),
    errorTitle: t(($) => $.alerts.deleteErrorTitle),
  })

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.contacts)}</Title>
        <ButtonLink to={contactsCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.contacts.addContact)}
        </ButtonLink>
      </Group>

      <PagedTable
        options={(query) => siteContactsListOptions({ path: { workspaceId }, query })}
        loadErrorTitle={t(($) => $.alerts.loadErrorTitle)}
        noRecordsText={t(($) => $.contacts.noRecords)}
        emptyState={t(($) => $.contacts.emptyState)}
        columns={[
          { accessor: 'email', title: t(($) => $.table.email) },
          { accessor: 'firstName', title: t(($) => $.table.firstName) },
          { accessor: 'lastName', title: t(($) => $.table.lastName) },
          { accessor: 'timeZone', title: t(($) => $.table.timeZone) },
          {
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Group gap="xs" wrap="nowrap">
                <Tooltip label={t(($) => $.actions.view)}>
                  <ActionIconLink
                    variant="light"
                    to={contactsDetailRoute.to}
                    params={{ workspaceId, contactId: record.id }}
                    aria-label={t(($) => $.actions.view)}
                  >
                    <IconEye size={16} />
                  </ActionIconLink>
                </Tooltip>
                <Tooltip label={t(($) => $.actions.edit)}>
                  <ActionIconLink
                    variant="light"
                    to={contactsEditRoute.to}
                    params={{ workspaceId, contactId: record.id }}
                    aria-label={t(($) => $.actions.edit)}
                  >
                    <IconPencil size={16} />
                  </ActionIconLink>
                </Tooltip>
                <Tooltip label={t(($) => $.actions.delete)}>
                  <ActionIcon
                    variant="light"
                    color="red"
                    onClick={() => deleteContact(record.id)}
                    aria-label={t(($) => $.actions.delete)}
                  >
                    <IconTrash size={16} />
                  </ActionIcon>
                </Tooltip>
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
