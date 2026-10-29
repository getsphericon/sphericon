import { EmptyState, Stack } from '@mantine/core'
import { useCounter } from '@mantine/hooks'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import type { QueryKey, UseQueryOptions } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import type { DataTableColumn } from 'mantine-datatable'
import { useDeferredValue, useState, ViewTransition } from 'react'
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
  // A different list in the same table (a filter or a view changed): back to the first page. The
  // table is not remounted, so its rows stay under the loading overlay until the new ones arrive.
  resetKey?: string
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
  resetKey,
}: PagedTableProps<TPage, TError, TKey>) {
  const [page, pageHandlers] = useCounter(1, { min: 1 })
  const [seenResetKey, setSeenResetKey] = useState(resetKey)
  if (seenResetKey !== resetKey) {
    setSeenResetKey(resetKey)
    pageHandlers.set(1)
  }
  const list = useQuery({ ...options({ page, pageSize }), placeholderData: keepPreviousData })
  // A deferred render is a React transition, which is what lets the ViewTransition below
  // cross-fade the old rows into the new ones when they arrive.
  const data = useDeferredValue(list.data)
  const totalItems = data?.totalItems ?? 0

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
            onPageChange={pageHandlers.set}
            fetching={list.isFetching}
            emptyState={<EmptyState title={noRecordsText} description={emptyState} />}
          />
        </ViewTransition>
      )}
    </Stack>
  )
}
