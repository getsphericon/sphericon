import { Badge, Group, Select, Stack, Text, TextInput, Title } from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { IconSearch } from '@tabler/icons-react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../components/ApiErrorAlert.tsx'
import { operatorWorkspacesListOptions } from '../generated/operator/@tanstack/react-query.gen.ts'
import type { OperatorWorkspaceReview } from '../generated/operator/types.gen.ts'
import { consoleWorkspaceRoute } from '../router.tsx'
import { formatDateTime } from '../utils/datetime.ts'

const PAGE_SIZE = 25
const REVIEWS: OperatorWorkspaceReview[] = ['warned', 'systemFrozen']

// The Workspace list: every tenant's metadata (never its Contacts, content or
// Events), searchable by slug, a row opens the Workspace.
export function ConsoleWorkspacesPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [search, setSearch] = useState('')
  const [debounced] = useDebouncedValue(search.trim(), 250)
  const [page, setPage] = useState(1)
  const [review, setReview] = useState<OperatorWorkspaceReview | null>(null)

  const workspacesQuery = useQuery({
    ...operatorWorkspacesListOptions({
      // An empty search is no filter: the request schema takes an absent key, not undefined.
      query: {
        page,
        pageSize: PAGE_SIZE,
        ...(debounced ? { slug: debounced } : {}),
        ...(review ? { review } : {}),
      },
    }),
    placeholderData: keepPreviousData,
  })

  return (
    <Stack>
      <Title order={2}>{t(($) => $.console.workspaces.title)}</Title>
      <Group>
        <TextInput
          aria-label={t(($) => $.console.workspaces.searchLabel)}
          placeholder={t(($) => $.console.workspaces.searchPlaceholder)}
          leftSection={<IconSearch size={16} />}
          value={search}
          onChange={(event) => {
            setSearch(event.currentTarget.value)
            setPage(1)
          }}
          w={{ base: '100%', sm: 320 }}
        />
        <Select
          aria-label={t(($) => $.console.workspaces.review.label)}
          placeholder={t(($) => $.console.workspaces.review.all)}
          clearable
          data={REVIEWS.map((r) => ({
            value: r,
            label: t(($) => $.console.workspaces.review[r]),
          }))}
          value={review}
          onChange={(value) => {
            setReview(REVIEWS.find((r) => r === value) ?? null)
            setPage(1)
          }}
          w={{ base: '100%', sm: 220 }}
        />
      </Group>
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
            render: (record) => (
              <Group gap="xs">
                {record.suspension ? (
                  <Badge color="red" variant="light">
                    {t(($) => $.console.workspaces.suspended)}
                  </Badge>
                ) : (
                  <Badge color="green" variant="light">
                    {t(($) => $.console.workspaces.active)}
                  </Badge>
                )}
                {record.warned ? (
                  <Badge color="yellow" variant="light">
                    {t(($) => $.console.workspaces.warned)}
                  </Badge>
                ) : null}
                {record.exempt ? (
                  <Badge color="grape" variant="light">
                    {t(($) => $.console.workspaces.exempt)}
                  </Badge>
                ) : null}
              </Group>
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
