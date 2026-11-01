import { EmptyState, Stack } from '@mantine/core'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import type { QueryKey, UseQueryOptions } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import type { DataTableColumn } from 'mantine-datatable'
import { useDeferredValue, ViewTransition } from 'react'
import type { ReactNode } from 'react'

import type { ApiErrorLike } from '../utils/apiErrors.ts'
import { ApiErrorAlert } from './ApiErrorAlert.tsx'
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
  noRecordsText: string
  // The EmptyState description shown while the list is empty, under the noRecordsText title.
  emptyState?: ReactNode
  // Opens the clicked row; the rows then highlight on hover.
  onRowClick?: (record: TPage['items'][number]) => void
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
  noRecordsText,
  emptyState,
  onRowClick,
  page = 1,
  onPageChange,
}: PagedTableProps<TPage, TError, TKey>) {
  const list = useQuery({ ...options({ page, pageSize }), placeholderData: keepPreviousData })
  // A deferred render is a React transition, which is what lets the ViewTransition below
  // cross-fade the old rows into the new ones when they arrive.
  const data = useDeferredValue(list.data)
  const totalItems = data?.totalItems ?? 0

  return (
    <Stack>
      {list.isError ? <ApiErrorAlert error={list.error} title={loadErrorTitle} /> : null}

      {/* While the first page loads only the skeleton shows: an empty table would flash its
          empty state under it. */}
      {list.isLoading ? (
        <TableSkeleton />
      ) : (
        <ViewTransition>
          <DataTable<TPage['items'][number]>
            minHeight={data?.items.length ? 0 : 200}
            withTableBorder
            {...(onRowClick
              ? { highlightOnHover: true, onRowClick: ({ record }) => onRowClick(record) }
              : {})}
            records={data?.items ?? []}
            columns={columns}
            totalRecords={totalItems}
            recordsPerPage={pageSize}
            page={page}
            onPageChange={onPageChange}
            fetching={list.isFetching}
            emptyState={<EmptyState title={noRecordsText} description={emptyState} />}
          />
        </ViewTransition>
      )}
    </Stack>
  )
}
