import { Loader, Stack, Text } from '@mantine/core'
import { useCounter } from '@mantine/hooks'
import { type QueryKey, type UseQueryOptions, useQuery } from '@tanstack/react-query'
import { DataTable, type DataTableColumn } from 'mantine-datatable'
import type { ReactNode } from 'react'

import type { ApiErrorLike } from '../utils/apiErrors.ts'
import { ApiErrorAlert } from './ApiErrorAlert.tsx'

type Page = { items: object[]; totalItems: number }

type PagedTableProps<TPage extends Page, TError extends ApiErrorLike, TKey extends QueryKey> = {
  // The generated list query options, called with the page to load.
  options: (paging: {
    page: number
    pageSize: number
  }) => UseQueryOptions<TPage, TError, TPage, TKey>
  columns: DataTableColumn<TPage['items'][number]>[]
  pageSize?: number
  loadErrorTitle: string
  noRecordsText: string
  // Shown under the table while the list is empty.
  emptyState?: ReactNode
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
}: PagedTableProps<TPage, TError, TKey>) {
  const [page, pageHandlers] = useCounter(1, { min: 1 })
  const list = useQuery(options({ page, pageSize }))
  const totalItems = list.data?.totalItems ?? 0

  return (
    <Stack>
      {list.isError ? (
        <ApiErrorAlert error={list.error} title={loadErrorTitle} fallback={loadErrorTitle} />
      ) : null}

      {list.isLoading ? <Loader /> : null}

      <DataTable<TPage['items'][number]>
        withTableBorder
        records={list.data?.items ?? []}
        columns={columns}
        totalRecords={totalItems}
        recordsPerPage={pageSize}
        page={page}
        onPageChange={pageHandlers.set}
        fetching={list.isFetching}
        noRecordsText={noRecordsText}
      />

      {emptyState && !list.isLoading && totalItems === 0 ? <Text>{emptyState}</Text> : null}
    </Stack>
  )
}
