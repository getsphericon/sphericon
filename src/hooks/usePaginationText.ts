import type { DataTableProps } from 'mantine-datatable'

import { useFormat } from './useFormat.ts'

type PaginationText = NonNullable<DataTableProps['paginationText']>

// usePaginationText is the "1-10/1234" under a table, written in the User's Regional format. The
// table prints its own range, which is plain digits unless it is given this.
export function usePaginationText(): PaginationText {
  const { formatNumber } = useFormat()
  return ({ from, to, totalRecords }) =>
    `${formatNumber(from)}-${formatNumber(to)}/${formatNumber(totalRecords)}`
}
