import { EmptyState, Stack } from '@mantine/core'
import { useCounter } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import type { QueryKey, UseQueryOptions } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import type { DataTableColumn } from 'mantine-datatable'
import type { ReactNode } from 'react'

import type { ApiErrorLike } from '../utils/apiErrors.ts'
import { ApiErrorAlert } from './ApiErrorAlert.tsx'
import { TableSkeleton } from './Skeletons.tsx'

interface Page {
  items: object[]
  totalItems: number
}

interface PagedTableProps<TPage extends Page, TError extends ApiErrorLike, TKey extends QueryKey> {
  // The generated list query options, called with the page to load.
  options: (paging: {
    page: number
    pageSize: number
  }) => UseQueryOptions<TPage, TError, TPage, TKey>
  columns: DataTableColumn<TPage['items'][number]>[]
  pageSize?: number
  loadErrorTitle: string
  noRecordsText: string
  // The EmptyState description shown while the list is empty, under the noRecordsText title.
  emptyState?: ReactNode
  // Opens the clicked row; the rows then highlight on hover.
  onRowClick?: (record: TPage['items'][number]) => void
}

// A server-paged mantine-datatable over a generated list query: owns the page, the load
// states and the error alert, so a list page only describes its columns.
export function PagedTable<TPage extends Page, TError extends ApiErrorLike, TKey extends QueryKey>({
  options,
  columns,
  pageSize = 10,
  loadErrorTitle,
  noRecordsText,
  emptyState,
  onRowClick,
}: PagedTableProps<TPage, TError, TKey>) {
  const [page, pageHandlers] = useCounter(1, { min: 1 })
  const list = useQuery(options({ page, pageSize }))
  const totalItems = list.data?.totalItems ?? 0

  return (
    <Stack>
      {list.isError ? (
        <ApiErrorAlert error={list.error} title={loadErrorTitle} fallback={loadErrorTitle} />
      ) : null}

      {/* While the first page loads only the skeleton shows: an empty table would flash its
          empty state under it. */}
      {list.isLoading ? (
        <TableSkeleton />
      ) : (
        <DataTable<TPage['items'][number]>
          minHeight={list.data?.items.length ? 0 : 200}
          withTableBorder
          {...(onRowClick
            ? { highlightOnHover: true, onRowClick: ({ record }) => onRowClick(record) }
            : {})}
          records={list.data?.items ?? []}
          columns={columns}
          totalRecords={totalItems}
          recordsPerPage={pageSize}
          page={page}
          onPageChange={pageHandlers.set}
          fetching={list.isFetching}
          emptyState={<EmptyState title={noRecordsText} description={emptyState} />}
        />
      )}
    </Stack>
  )
}
