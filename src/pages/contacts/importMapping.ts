import * as z from 'zod'

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
import {
  consent,
  consentChannels,
  engagementTracking,
  factColumnOf,
  factOf,
  factTargets,
  time,
} from './importFacts.ts'
import type { FactSettings } from './importFacts.ts'

// What a column of a CSV import writes, as the value of its type control: `ignore`, `tags`,
// `key:<key>`, `attribute:<attribute>`, `customField:<key>`, `newCustomField`, or a fact kind of
// importFacts.ts (`refusal`, `consent`, `engagementTracking`, `time`) (ADR 0138). The control and the
// mapping a request saves are both read from it, so the two never disagree.
export type Target = string

// A Custom field a column may write: its key and its type.
export interface FieldOption {
  key: string
  type: CustomFieldType
}

// A Custom field a column creates when the import runs: its key, label and type, which the
// Member chooses.
export interface NewFieldChoice {
  key: string
  label: string
  type: CustomFieldType
}

// A column as the mapping form holds it: what it writes, for a date its format and, for a new
// Custom field, the field.
export interface ColumnChoice {
  target: Target
  dateFormat: ContactImportDateFormat | null
  newField?: NewFieldChoice
  // What a refusal, consent, tracking or time column records; undefined for any other target.
  fact?: FactSettings | undefined
}

// The mapping form's values: a choice per column of the file and the choices for every row, the
// Tag for every row as typed ('' for none).
export type MappingValues = Omit<ContactImportMappingInput, 'columns' | 'tag'> & {
  columns: ColumnChoice[]
  tag: string
}

export const ignore: Target = 'ignore'
export const tags: Target = 'tags'
export const newCustomField: Target = 'newCustomField'

// The new Custom field proposed for a column: its header as the label, the header in lower case
// with every run of other characters than letters and digits an underscore as the key, and text.
export function newFieldFor(header: string): NewFieldChoice {
  const label = header.trim()
  const key = label
    .toLowerCase()
    .replaceAll(/[^\p{L}\p{N}]+/gu, '_')
    .replaceAll(/^_+|_+$/g, '')
  return { key, label, type: 'string' }
}

// Whether a column's choice writes a date: a new datetime field, or a target that writes one.
export function choiceWritesDate(choice: ColumnChoice, fields: readonly FieldOption[]): boolean {
  if (choice.target === newCustomField) {
    return choice.newField?.type === 'datetime'
  }
  return writesDate(choice.target, fields)
}

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
function targetOf(column: ContactImportColumn): Target {
  if (column.kind === 'key') {
    return `key:${column.key}`
  }
  if (column.kind === 'attribute') {
    return `attribute:${column.attribute}`
  }
  if (column.kind === 'customField') {
    return `customField:${column.key}`
  }
  if (column.kind === 'newCustomField') {
    return newCustomField
  }
  if (factTargets.includes(column.kind)) {
    return column.kind
  }
  return column.kind === 'tags' ? tags : ignore
}

// Whether a target writes a date, which needs its format: the birthday, or a datetime Custom field.
export function writesDate(target: Target, fields: readonly FieldOption[]): boolean {
  if (target === 'attribute:birthday' || target === time) {
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
      fact: factOf(column),
      ...(column.kind === 'newCustomField'
        ? { newField: { key: column.key, label: column.label, type: column.type } }
        : {}),
    })),
    decimalSeparator: mapping.decimalSeparator,
    defaultCountry: mapping.defaultCountry,
    tag: mapping.tag ?? '',
    onlyAddNew: mapping.onlyAddNew,
    asNewSignUps: mapping.asNewSignUps,
  }
}

// The column a request saves for a choice; a date format is sent with a date column only.
function columnOf(choice: ColumnChoice, fields: readonly FieldOption[]): ContactImportColumnInput {
  const dateFormat = choiceWritesDate(choice, fields) ? choice.dateFormat : null
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
  if (kind === newCustomField) {
    const field = choice.newField ?? newFieldFor('')
    return { kind: 'newCustomField', ...field, dateFormat }
  }
  const fact = choice.fact === undefined ? undefined : factColumnOf(kind, choice.fact, dateFormat)
  if (fact !== undefined) {
    return fact
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
    asNewSignUps: values.asNewSignUps,
  }
}

// The columns that write a date and have no format yet: the import cannot run until the Member
// chooses one for each, since an ambiguous date is never guessed.
export function columnsMissingDateFormat(
  values: MappingValues,
  fields: readonly FieldOption[],
): number[] {
  return values.columns.flatMap((choice, at) =>
    choiceWritesDate(choice, fields) && choice.dateFormat === null ? [at] : [],
  )
}

// The messages of the form's own checks.
export interface MappingMessages {
  // A date column without its format.
  dateFormat: string
  // A consent or tracking column left on a Channel that has none (Telegram).
  consentChannel: string
}

// The mapping form's schema: a date column needs its format, and a consent or tracking column a
// Channel that has them, each refused under its column's control; a valid form is the mapping a
// request saves. What the contract bounds is the server's to refuse, which the form shows under
// the column it names (columnFieldOf).
export function mappingSchema(
  fields: readonly FieldOption[],
  messages: MappingMessages,
): z.ZodType<ContactImportMappingInput, MappingValues> {
  return z
    .custom<MappingValues>()
    .superRefine((values, ctx) => {
      for (const at of columnsMissingDateFormat(values, fields)) {
        ctx.addIssue({
          code: 'custom',
          path: ['columns', at, 'dateFormat'],
          message: messages.dateFormat,
        })
      }
      for (const [at, choice] of values.columns.entries()) {
        const onConsentChannel =
          choice.fact === undefined || consentChannels[choice.fact.channel] !== null
        if (
          (choice.target === consent || choice.target === engagementTracking) &&
          !onConsentChannel
        ) {
          ctx.addIssue({
            code: 'custom',
            path: ['columns', at, 'target'],
            message: messages.consentChannel,
          })
        }
      }
    })
    .transform((values) => mappingInputOf(values, fields))
}

// The form's field a refusal of the save names by its path: a column's (`columns.<n>`, or a part
// of it) is its date format when that is what it lacks, else its type control; any other path is
// the form's field of the same name.
export function columnFieldOf(
  path: string,
  values: MappingValues,
  fields: readonly FieldOption[],
): string {
  const match = /^columns\.(\d+)(?:\.|$)/.exec(path)
  if (match === null) {
    return path
  }
  const at = Number(match[1])
  const choice = values.columns[at]
  const lacksFormat =
    choice !== undefined && choiceWritesDate(choice, fields) && choice.dateFormat === null
  return lacksFormat ? `columns.${at}.dateFormat` : `columns.${at}.target`
}
