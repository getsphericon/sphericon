import { Avatar, Badge, Button, Group, Stack, Text, TextInput, Title } from '@mantine/core'
import { useDebouncedCallback, useDisclosure } from '@mantine/hooks'
import { IconSearch } from '@tabler/icons-react'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CountryFlag } from '../../components/CountryFlag.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { RelativeTime } from '../../components/RelativeTime.tsx'
import { MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { siteContactsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/edit.tsx'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { withPage } from '../../searchSchemas.ts'
import { contactFullName, contactName } from '../../utils/contactName.ts'
import { ContactCreateModal } from './resource.tsx'

export function ContactsListPage() {
  const pageNavigate = contactsRoute.useNavigate()
  const { t } = useTranslation()
  const { workspaceId } = contactsRoute.useParams()
  const { email, page } = contactsRoute.useSearch()
  const navigate = useNavigate({ from: contactsRoute.fullPath })
  const [createOpened, { open: openCreate, close: closeCreate }] = useDisclosure(false)
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
      <ContactCreateModal opened={createOpened} onClose={closeCreate} />
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.contacts)}</Title>
        <Button onClick={openCreate}>{t(($) => $.contacts.addContact)}</Button>
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
        page={page}
        onPageChange={(next) =>
          void pageNavigate({ search: withPage(next), viewTransition: false })
        }
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
        onRowClick={(record) =>
          void navigate({
            to: contactsDetailRoute.to,
            params: { workspaceId, contactId: record.id },
          })
        }
        columns={[
          {
            // Who the Contact is in one cell: the name, with the email under it; a Contact
            // without a name shows its first identifier alone.
            accessor: 'email',
            title: t(($) => $.table.contact),
            render: (record) => {
              const fullName = contactFullName(record)
              const identifier = record.email ?? record.subjectId ?? record.phone
              return (
                <Group gap="sm" wrap="nowrap">
                  <Avatar
                    size="sm"
                    color="initials"
                    name={contactName(record)}
                    src={record.avatar ?? null}
                    alt={contactName(record)}
                  />
                  <Stack gap={0}>
                    <Group gap={6} wrap="nowrap">
                      <Text size="sm" fw={500}>
                        {fullName || identifier}
                      </Text>
                      {record.country ? <CountryFlag code={record.country} /> : null}
                    </Group>
                    {fullName && identifier ? (
                      <Text size="xs" c="dimmed">
                        {identifier}
                      </Text>
                    ) : null}
                  </Stack>
                </Group>
              )
            },
          },
          {
            accessor: 'tags',
            title: t(($) => $.table.tags),
            render: (record) => (
              <Group gap={4}>
                {record.tags.map((tag) => (
                  <Badge key={tag} variant="light" size="sm">
                    {tag}
                  </Badge>
                ))}
              </Group>
            ),
          },
          {
            accessor: 'createdAt',
            title: t(($) => $.table.added),
            render: (record) => <RelativeTime value={record.createdAt} />,
          },
          {
            accessor: 'actions',
            title: t(($) => $.table.actions),
            render: (record) => (
              <Group justify="flex-end">
                <RowActionsMenu>
                  <MenuItemLink
                    to={contactsDetailRoute.to}
                    params={{ workspaceId, contactId: record.id }}
                  >
                    {t(($) => $.actions.view)}
                  </MenuItemLink>
                  <MenuItemLink
                    to={contactsEditRoute.to}
                    params={{ workspaceId, contactId: record.id }}
                  >
                    {t(($) => $.actions.edit)}
                  </MenuItemLink>
                </RowActionsMenu>
              </Group>
            ),
          },
        ]}
      />
    </Stack>
  )
}
