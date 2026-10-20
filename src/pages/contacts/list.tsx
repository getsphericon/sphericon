import { Group, Stack, Title, Tooltip } from '@mantine/core'
import { IconEye, IconPencil } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ActionIconLink, ButtonLink } from '../../components/RouterLink.tsx'
import { siteContactsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$slug/contacts/$contactId/edit.tsx'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$slug/contacts/$contactId/index.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$slug/contacts/index.tsx'
import { Route as contactsCreateRoute } from '../../routes/workspaces/$slug/contacts/new.tsx'

export function ContactsListPage() {
  const { t } = useTranslation()
  const { slug } = contactsRoute.useParams()

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.contacts)}</Title>
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
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
