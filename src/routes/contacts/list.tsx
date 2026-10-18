import { ActionIcon, Group, Stack, Tooltip } from '@mantine/core'
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
import {
  contactsCreateRoute,
  contactsDetailRoute,
  contactsEditRoute,
  contactsRoute,
} from '../../router.tsx'

export function ContactsListPage() {
  const { t } = useTranslation()
  const { slug } = contactsRoute.useParams()

  const deleteContact = useDeleteResource({
    mutation: siteContactsDeleteMutation(),
    slug,
    listQueryKey: siteContactsListQueryKey({ path: { slug } }),
    successMessage: t(($) => $.notifications.contactDeleted),
    errorTitle: t(($) => $.alerts.deleteErrorTitle),
  })

  return (
    <Stack>
      <Group justify="flex-end" align="flex-end" wrap="wrap">
        <ButtonLink to={contactsCreateRoute.to} params={{ slug }}>
          {t(($) => $.contacts.addContact)}
        </ButtonLink>
      </Group>

      <PagedTable
        options={(query) => siteContactsListOptions({ path: { slug }, query })}
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
                    params={{ slug, contactId: record.id }}
                    aria-label={t(($) => $.actions.view)}
                  >
                    <IconEye size={16} />
                  </ActionIconLink>
                </Tooltip>
                <Tooltip label={t(($) => $.actions.edit)}>
                  <ActionIconLink
                    variant="light"
                    to={contactsEditRoute.to}
                    params={{ slug, contactId: record.id }}
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
