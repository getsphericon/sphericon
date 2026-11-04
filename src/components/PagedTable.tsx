import { ActionIcon, EmptyState, Group, Stack, Text, Title, Tooltip } from '@mantine/core'
import { IconRefresh } from '@tabler/icons-react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import type { QueryKey, UseQueryOptions } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import type { DataTableColumn } from 'mantine-datatable'
import { useDeferredValue, ViewTransition } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { usePaginationText } from '../hooks/usePaginationText.ts'
import type { ApiErrorLike } from '../utils/apiErrors.ts'
import { ApiErrorAlert } from './ApiErrorAlert.tsx'
import { CenteredEmptyState } from './CenteredEmptyState.tsx'
import { RelativeTime } from './RelativeTime.tsx'
import { TableSkeleton } from './Skeletons.tsx'

interface Page {
  items: object[]
  totalItems: number
}

interface PagedTableProps<TPage extends Page, TError extends ApiErrorLike, TKey extends QueryKey> {
  // The page shown, the route's `page` search param (pageParam), and the move to another one
  // (a navigation with withPage).
  page: number | undefined
  onPageChange: (page: number) => void
  // The generated list query options, called with the page to load.
  options: (paging: {
    page: number
    pageSize: number
  }) => UseQueryOptions<TPage, TError, TPage, TKey>
  columns: DataTableColumn<TPage['items'][number]>[]
  pageSize?: number
  loadErrorTitle: string
  // The collection's name; the toolbar shows it with the total ("Contacts (0)").
  collection: string
  // What the table shows when it has no rows: a heading naming the state, a description only when
  // it says why or what to do, and one action. An empty list invites the create action; zero
  // results from a filter offer to clear it (docs/standards/ui.md, "Empty states").
  empty: {
    title: string
    description?: string
    action?: ReactNode
  }
  // Opens the clicked row; the rows then highlight on hover.
  onRowClick?: (record: TPage['items'][number]) => void
  // A list that is the whole page puts its page header here (Cloudscape's full-page table): the
  // collection's name is the page title with `actions` (the create button) at its right, so the
  // table does not repeat the title above itself. `toolbar` (the archived view, the filters) and
  // the count of what it shows share the row of the refresh button.
  pageHeader?: PageHeader
}

interface PageHeader {
  actions?: ReactNode
  toolbar?: ReactNode
  // The count of what the view shows ("6 automations"), beside the toolbar.
  count: (total: number) => string
}

// A server-paged mantine-datatable over a generated list query: owns the page, the load
// states and the error alert, so a list page only describes its columns. The page lives in the
// route's `page` search param, so it is linkable and Back returns to it; a link that changes the
// list (a filter, the archived view) leaves the param out and lands on the first page.
export function PagedTable<TPage extends Page, TError extends ApiErrorLike, TKey extends QueryKey>({
  options,
  columns,
  pageSize = 10,
  loadErrorTitle,
  collection,
  empty,
  onRowClick,
  page = 1,
  onPageChange,
  pageHeader,
}: PagedTableProps<TPage, TError, TKey>) {
  const paginationText = usePaginationText()
  const list = useQuery({ ...options({ page, pageSize }), placeholderData: keepPreviousData })
  // A deferred render is a React transition, which is what lets the ViewTransition below
  // cross-fade the old rows into the new ones when they arrive.
  const data = useDeferredValue(list.data)
  const totalItems = data?.totalItems ?? 0
  // The time of the last successful fetch: a failed refresh leaves it, and a placeholder page
  // shown while another loads has none (0), so the time then waits for the page's own fetch.
  const refreshedAt = list.dataUpdatedAt > 0 ? new Date(list.dataUpdatedAt).toISOString() : null
  // A list with nothing in it shows its empty state in place of the table, centered in the page.
  // It is the total that says so, not the page: a later page can come back empty while the list
  // is not, and its table keeps the pagination back.
  const nothing = data !== undefined && totalItems === 0

  return (
    // It grows into the page's free height, so an empty list's state sits in the page's middle.
    <Stack flex={1}>
      <PagedTableHeader
        collection={collection}
        total={data?.totalItems}
        pageHeader={pageHeader}
        refreshedAt={refreshedAt}
        fetching={list.isFetching}
        onRefresh={() => void list.refetch()}
      />

      {list.isError ? <ApiErrorAlert error={list.error} title={loadErrorTitle} /> : null}

      {nothing ? (
        <CenteredEmptyState title={empty.title} description={empty.description}>
          {empty.action ? <EmptyState.Actions>{empty.action}</EmptyState.Actions> : null}
        </CenteredEmptyState>
      ) : null}

      {/* Until the first page is on screen only the skeleton shows. It waits for the deferred
          rows, not the request: the request settles a render before they arrive, and a table
          drawn in that render would flash its empty state. A failed first load shows the alert
          alone, since an error is not an empty list. */}
      {data === undefined && !list.isError ? <TableSkeleton /> : null}
      {data === undefined || nothing ? null : (
        <ViewTransition>
          <DataTable<TPage['items'][number]>
            minHeight={data.items.length > 0 ? 0 : 200}
            withTableBorder
            // Striped, so the eye keeps its row across the columns (docs/standards/ui.md).
            striped
            {...(onRowClick
              ? { highlightOnHover: true, onRowClick: ({ record }) => onRowClick(record) }
              : {})}
            records={data.items}
            columns={columns}
            totalRecords={totalItems}
            paginationText={paginationText}
            recordsPerPage={pageSize}
            page={page}
            onPageChange={onPageChange}
            fetching={list.isFetching}
            emptyState={
              <EmptyState title={empty.title} description={empty.description}>
                {empty.action ? <EmptyState.Actions>{empty.action}</EmptyState.Actions> : null}
              </EmptyState>
            }
          />
        </ViewTransition>
      )}
    </Stack>
  )
}

// The rows above the table. A full-page list puts its page header first, the collection's name as
// the title and the actions at its right, then its toolbar and count beside the refresh. Other
// lists name the collection with its count where the toolbar goes.
function PagedTableHeader({
  collection,
  total,
  pageHeader,
  refreshedAt,
  fetching,
  onRefresh,
}: {
  collection: string
  total: number | undefined
  pageHeader: PageHeader | undefined
  refreshedAt: string | null
  fetching: boolean
  onRefresh: () => void
}) {
  const { t } = useTranslation()
  const refreshLabel = t(($) => $.table.refresh)
  return (
    <>
      {pageHeader ? (
        // The page's title stands apart from the toolbar and the table under it.
        <Group justify="space-between" align="center" mb="md">
          <Title order={2}>{collection}</Title>
          {pageHeader.actions}
        </Group>
      ) : null}
      <Group justify="space-between" gap="xs">
        {pageHeader ? (
          <Group gap="md">
            {pageHeader.toolbar}
            {/* The count of the view on screen (the archived one counts its own), once the first
                page is in: a zero before it would be a guess. */}
            {total === undefined ? null : (
              <Text size="sm" c="dimmed">
                {pageHeader.count(total)}
              </Text>
            )}
          </Group>
        ) : (
          <Text fw={600}>
            {t(($) => $.table.collection, { name: collection, total: total ?? 0 })}
          </Text>
        )}
        <Group gap="xs">
          {/* Announced politely each time it changes, so a screen reader hears that the refresh
            landed without the focus moving. */}
          <Text component="output" size="sm" c="dimmed" aria-live="polite">
            {refreshedAt ? (
              <>
                {t(($) => $.table.updated)} <RelativeTime value={refreshedAt} />
              </>
            ) : null}
          </Text>
          <Tooltip label={refreshLabel} withArrow>
            <ActionIcon
              variant="subtle"
              color="gray"
              aria-label={refreshLabel}
              loading={fetching}
              onClick={onRefresh}
            >
              <IconRefresh size={16} />
            </ActionIcon>
          </Tooltip>
        </Group>
      </Group>
    </>
  )
}
