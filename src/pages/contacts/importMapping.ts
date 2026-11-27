import type {
  ContactImportAttribute,
  ContactImportColumn,
  ContactImportColumnInput,
  ContactImportDateFormat,
  ContactImportKey,
  ContactImportMapping,
  ContactImportMappingInput,
  CustomFieldType,
} from '../../generated/site/types.gen.ts'
import { zContactImportAttribute, zContactImportKey } from '../../generated/site/zod.gen.ts'

// What a column of a CSV import writes, as the value of its type control: `ignore`, `tags`,
// `key:<key>`, `attribute:<attribute>` or `customField:<key>` (ADR 0138). The control and the
// mapping a request saves are both read from it, so the two never disagree.
export type Target = string

// A Custom field a column may write: its key and its type.
export interface FieldOption {
  key: string
  type: CustomFieldType
}

// A column as the mapping form holds it: what it writes and, for a date, its format.
export interface ColumnChoice {
  target: Target
  dateFormat: ContactImportDateFormat | null
}

// The mapping form's values: a choice per column of the file and the choices for every row.
export interface MappingValues {
  columns: ColumnChoice[]
  decimalSeparator: ContactImportMappingInput['decimalSeparator']
  defaultCountry: string | null
  tag: string
  onlyAddNew: boolean
}

export const ignore: Target = 'ignore'
export const tags: Target = 'tags'

// The targets a column may write, in the control's order: the alias keys, the core attributes,
// then the Workspace's Custom fields by key, each with what it names.
export function targets(fields: readonly FieldOption[]): {
  keys: { value: Target; key: ContactImportKey }[]
  attributes: { value: Target; attribute: ContactImportAttribute }[]
  customFields: { value: Target; key: string }[]
} {
  return {
    keys: zContactImportKey.options.map((key) => ({ value: `key:${key}`, key })),
    attributes: zContactImportAttribute.options.map((attribute) => ({
      value: `attribute:${attribute}`,
      attribute,
    })),
    customFields: fields.map((field) => ({ value: `customField:${field.key}`, key: field.key })),
  }
}

// The target a column of a mapping writes.
export function targetOf(column: ContactImportColumn): Target {
  if (column.kind === 'key') {
    return `key:${column.key}`
  }
  if (column.kind === 'attribute') {
    return `attribute:${column.attribute}`
  }
  if (column.kind === 'customField') {
    return `customField:${column.key}`
  }
  return column.kind === 'tags' ? tags : ignore
}

// Whether a target writes a date, which needs its format: the birthday, or a datetime Custom field.
export function writesDate(target: Target, fields: readonly FieldOption[]): boolean {
  if (target === 'attribute:birthday') {
    return true
  }
  const key = target.startsWith('customField:') ? target.slice('customField:'.length) : undefined
  return fields.some((field) => field.key === key && field.type === 'datetime')
}

// The form's values for a mapping the server answered (the saved one, or its proposal).
export function valuesOf(mapping: ContactImportMapping): MappingValues {
  return {
    columns: mapping.columns.map((column) => ({
      target: targetOf(column),
      dateFormat: 'dateFormat' in column ? column.dateFormat : null,
    })),
    decimalSeparator: mapping.decimalSeparator,
    defaultCountry: mapping.defaultCountry,
    tag: mapping.tag ?? '',
    onlyAddNew: mapping.onlyAddNew,
  }
}

// The column a request saves for a choice; a date format is sent with a date column only.
export function columnOf(
  choice: ColumnChoice,
  fields: readonly FieldOption[],
): ContactImportColumnInput {
  const dateFormat = writesDate(choice.target, fields) ? choice.dateFormat : null
  const at = choice.target.indexOf(':')
  const kind = at < 0 ? choice.target : choice.target.slice(0, at)
  const name = at < 0 ? '' : choice.target.slice(at + 1)
  if (kind === 'key') {
    return { kind: 'key', key: zContactImportKey.parse(name) }
  }
  if (kind === 'attribute') {
    return { kind: 'attribute', attribute: zContactImportAttribute.parse(name), dateFormat }
  }
  if (kind === 'customField') {
    return { kind: 'customField', key: name, dateFormat }
  }
  return kind === 'tags' ? { kind: 'tags' } : { kind: 'ignore' }
}

// The mapping a request saves for the form's values.
export function mappingInputOf(
  values: MappingValues,
  fields: readonly FieldOption[],
): ContactImportMappingInput {
  return {
    columns: values.columns.map((choice) => columnOf(choice, fields)),
    decimalSeparator: values.decimalSeparator,
    defaultCountry: values.defaultCountry,
    tag: values.tag.trim() === '' ? null : values.tag.trim(),
    onlyAddNew: values.onlyAddNew,
  }
}

// The columns that write a date and have no format yet: the import cannot run until the Member
// chooses one for each, since an ambiguous date is never guessed.
export function columnsMissingDateFormat(
  values: MappingValues,
  fields: readonly FieldOption[],
): number[] {
  return values.columns.flatMap((choice, at) =>
    writesDate(choice.target, fields) && choice.dateFormat === null ? [at] : [],
  )
}
