import { Badge, Stack, Text, TextInput, Title } from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { IconSearch } from '@tabler/icons-react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../components/ApiErrorAlert.tsx'
import { operatorWorkspacesListOptions } from '../generated/operator/@tanstack/react-query.gen.ts'
import { consoleWorkspaceRoute } from '../router.tsx'
import { formatDateTime } from '../utils/datetime.ts'

const PAGE_SIZE = 25

// The Workspace list: every tenant's metadata (never its Contacts, content or
// Events), searchable by slug, a row opens the Workspace.
export function ConsoleWorkspacesPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [search, setSearch] = useState('')
  const [debounced] = useDebouncedValue(search.trim(), 250)
  const [page, setPage] = useState(1)

  const workspacesQuery = useQuery({
    ...operatorWorkspacesListOptions({
      // An empty search is no filter: the request schema takes an absent key, not undefined.
      query: { page, pageSize: PAGE_SIZE, ...(debounced ? { slug: debounced } : {}) },
    }),
    placeholderData: keepPreviousData,
  })

  return (
    <Stack>
      <Title order={2}>{t(($) => $.console.workspaces.title)}</Title>
      <TextInput
        aria-label={t(($) => $.console.workspaces.searchLabel)}
        placeholder={t(($) => $.console.workspaces.searchPlaceholder)}
        leftSection={<IconSearch size={16} />}
        value={search}
        onChange={(event) => {
          setSearch(event.currentTarget.value)
          setPage(1)
        }}
        maw={400}
      />
      {workspacesQuery.isError ? (
        <ApiErrorAlert
          error={workspacesQuery.error}
          title={t(($) => $.console.workspaces.loadError)}
          fallback={t(($) => $.console.workspaces.loadError)}
        />
      ) : null}
      <DataTable
        withTableBorder
        highlightOnHover
        fetching={workspacesQuery.isFetching}
        records={workspacesQuery.data?.items ?? []}
        idAccessor="id"
        columns={[
          { accessor: 'slug', title: t(($) => $.console.workspaces.slug) },
          { accessor: 'name', title: t(($) => $.console.workspaces.name) },
          {
            accessor: 'createdAt',
            title: t(($) => $.console.workspaces.createdAt),
            visibleMediaQuery: (theme) => `(min-width: ${theme.breakpoints.sm})`,
            render: (record) => <Text size="sm">{formatDateTime(record.createdAt)}</Text>,
          },
          {
            accessor: 'suspension',
            title: t(($) => $.console.workspaces.status),
            render: (record) =>
              record.suspension ? (
                <Badge color="red" variant="light">
                  {t(($) => $.console.workspaces.suspended)}
                </Badge>
              ) : (
                <Badge color="green" variant="light">
                  {t(($) => $.console.workspaces.active)}
                </Badge>
              ),
          },
        ]}
        onRowClick={({ record }) =>
          navigate({ to: consoleWorkspaceRoute.to, params: { workspaceId: record.id } })
        }
        noRecordsText={t(($) => $.console.workspaces.empty)}
        totalRecords={workspacesQuery.data?.totalItems ?? 0}
        recordsPerPage={PAGE_SIZE}
        page={page}
        onPageChange={setPage}
      />
    </Stack>
  )
}
