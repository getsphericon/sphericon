import { Avatar, Badge, Button, Group, Select, Stack, Text, TextInput, Title } from '@mantine/core'
import { useDebouncedCallback, useDisclosure } from '@mantine/hooks'
import { IconSearch } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CountryFlag } from '../../components/CountryFlag.tsx'
import { PagedTable } from '../../components/PagedTable.tsx'
import { RelativeTime } from '../../components/RelativeTime.tsx'
import { MenuItemLink } from '../../components/RouterLink.tsx'
import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteContactsListOptions,
  siteTagsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as contactsEditRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/edit.tsx'
import { Route as contactsDetailRoute } from '../../routes/workspaces/$workspaceId/contacts/$contactId/index.tsx'
import { Route as contactsRoute } from '../../routes/workspaces/$workspaceId/contacts/index.tsx'
import { withPage } from '../../searchSchemas.ts'
import { contactFullName, contactName } from '../../utils/contactName.ts'
import { countryOptions } from '../../utils/countries.ts'
import { ContactCreateModal } from './resource.tsx'

// The Workspace's Tags to filter by (the first hundred). The Tag in the URL is always an option, so a link to a Tag
// the first page of Tags misses still shows what it filters by.
function TagFilter({
  workspaceId,
  value,
  onChange,
}: {
  workspaceId: string
  value: string | undefined
  onChange: (next: string | undefined) => void
}) {
  const { t } = useTranslation()
  // Fetched once the dropdown first opens: a list read for its contacts needs no Tags.
  const [opened, setOpened] = useState(false)
  const tags = useQuery({
    ...siteTagsListOptions({ path: { workspaceId }, query: { pageSize: 100 } }),
    enabled: opened,
  })
  const names = (tags.data?.items ?? []).map((item) => item.name)
  const data = value && !names.includes(value) ? [value, ...names] : names
  return (
    <Select
      w={200}
      searchable
      clearable
      data={data}
      value={value ?? null}
      onChange={(next) => onChange(next ?? undefined)}
      onDropdownOpen={() => setOpened(true)}
      placeholder={t(($) => $.contacts.tagFilter)}
      aria-label={t(($) => $.contacts.tagFilter)}
    />
  )
}

export function ContactsListPage() {
  const pageNavigate = contactsRoute.useNavigate()
  const { t } = useTranslation()
  const { workspaceId } = contactsRoute.useParams()
  const { email, tag, country, page } = contactsRoute.useSearch()
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
  // A filter changes only its own key of the search and starts again from the first page.
  const setFilter = (patch: {
    email?: string | undefined
    tag?: string | undefined
    country?: string | undefined
  }) => void navigate({ search: (prev) => ({ ...prev, ...patch, page: undefined }), replace: true })
  const applySearch = useDebouncedCallback((value: string) => {
    setFilter({ email: value.trim() || undefined })
  }, 300)
  const filtered = Boolean(email || tag || country)

  return (
    <Stack>
      <ContactCreateModal opened={createOpened} onClose={closeCreate} />
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.nav.contacts)}</Title>
        <Button onClick={openCreate}>{t(($) => $.contacts.addContact)}</Button>
      </Group>

      <Group gap="sm" align="flex-start">
        <TextInput
          flex={1}
          miw={200}
          value={search}
          onChange={(event) => {
            setSearch(event.currentTarget.value)
            applySearch(event.currentTarget.value)
          }}
          placeholder={t(($) => $.contacts.searchPlaceholder)}
          aria-label={t(($) => $.contacts.searchPlaceholder)}
          leftSection={<IconSearch size={16} />}
        />
        <TagFilter
          workspaceId={workspaceId}
          value={tag}
          onChange={(next) => setFilter({ tag: next })}
        />
        <Select
          w={220}
          searchable
          clearable
          data={countryOptions}
          value={country ?? null}
          onChange={(next) => setFilter({ country: next ?? undefined })}
          placeholder={t(($) => $.contacts.countryFilter)}
          aria-label={t(($) => $.contacts.countryFilter)}
          leftSection={country ? <CountryFlag code={country} /> : null}
        />
      </Group>

      <PagedTable
        page={page}
        onPageChange={(next) =>
          void pageNavigate({ search: withPage(next), viewTransition: false })
        }
        // A new search or filter starts again from the first page.
        key={[email, tag, country].join('\u0000')}
        options={(query) =>
          siteContactsListOptions({
            path: { workspaceId },
            query: {
              ...query,
              ...(email ? { email } : {}),
              ...(tag ? { tag } : {}),
              ...(country ? { country } : {}),
            },
          })
        }
        loadErrorTitle={t(($) => $.alerts.loadErrorTitle)}
        // A search or filter that matches nothing is not an empty Workspace: no invitation to
        // create the first contact.
        noRecordsText={filtered ? t(($) => $.contacts.noMatches) : t(($) => $.contacts.noRecords)}
        emptyState={filtered ? undefined : t(($) => $.contacts.emptyState)}
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
                {record.tags.map((name) => (
                  <Badge key={name} variant="light" size="sm">
                    {name}
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
