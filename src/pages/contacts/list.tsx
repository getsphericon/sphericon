import { Group, Stack, TextInput, Title, Tooltip } from '@mantine/core'
import { useDebouncedCallback } from '@mantine/hooks'
import { IconEye, IconPencil, IconSearch } from '@tabler/icons-react'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import { ActionIconLink, ButtonLink } from '../../components/RouterLink.tsx'
import { siteContactsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/edit.tsx'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { Route as contactsCreateRoute } from '../../routes/workspaces/$workspaceId/contacts/new.tsx'

export function ContactsListPage() {
  const { t } = useTranslation()
  const { workspaceId } = contactsRoute.useParams()
  const { email } = contactsRoute.useSearch()
  const navigate = useNavigate({ from: contactsRoute.fullPath })
  const [search, setSearch] = useState(email ?? '')
  // Back/forward or a link changes the URL search: the field follows it, unless it already
  // shows that search (typing trims before it navigates).
  const [seenEmail, setSeenEmail] = useState(email)
  if (email !== seenEmail) {
    setSeenEmail(email)
    if (search.trim() !== (email ?? '')) {
      setSearch(email ?? '')
    }
  }
  const applySearch = useDebouncedCallback((value: string) => {
    void navigate({ search: { email: value.trim() || undefined }, replace: true })
  }, 300)

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.contacts)}</Title>
        <ButtonLink to={contactsCreateRoute.to} params={{ workspaceId }}>
          {t(($) => $.contacts.addContact)}
        </ButtonLink>
      </Group>

      <TextInput
        value={search}
        onChange={(event) => {
          setSearch(event.currentTarget.value)
          applySearch(event.currentTarget.value)
        }}
        placeholder={t(($) => $.contacts.searchPlaceholder)}
        aria-label={t(($) => $.contacts.searchPlaceholder)}
        leftSection={<IconSearch size={16} />}
      />

      <PagedTable
        // A new search starts again from the first page.
        key={email ?? ''}
        options={(query) =>
          siteContactsListOptions({
            path: { workspaceId },
            query: email ? { ...query, email } : query,
          })
        }
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
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
