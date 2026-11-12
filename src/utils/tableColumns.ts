import type { DataTableColumn } from 'mantine-datatable'

// A column that shows a field of the record as it is, with no render of its own. DataTable takes
// any string as an accessor, so a misspelt field compiles and shows an empty column: a column
// written `{ accessor, title } satisfies FieldColumn<Record>` takes only a field of the record,
// and leaves the table's inference of its records as it was.
export type FieldColumn<T> = Omit<DataTableColumn<T>, 'accessor' | 'render'> & {
  accessor: Extract<keyof T, string>
}
