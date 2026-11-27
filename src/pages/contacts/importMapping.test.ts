import { expect, test } from 'vitest'

import type { ContactImportMapping } from '../../generated/site/types.gen.ts'
import {
  columnsMissingDateFormat,
  mappingInputOf,
  targets,
  valuesOf,
  writesDate,
} from './importMapping.ts'

const fields = [
  { key: 'plan', type: 'string' as const },
  { key: 'renews_at', type: 'datetime' as const },
]

const proposal: ContactImportMapping = {
  columns: [
    { kind: 'key', key: 'email' },
    { kind: 'attribute', attribute: 'birthday', dateFormat: null },
    { kind: 'customField', key: 'renews_at', dateFormat: 'dd/mm/yyyy' },
    { kind: 'tags' },
    { kind: 'ignore' },
  ],
  decimalSeparator: ',',
  defaultCountry: null,
  tag: 'Imported 2026-11-27',
  onlyAddNew: false,
}

test('a mapping read back is the mapping a request saves', () => {
  const input = mappingInputOf(valuesOf(proposal), fields)

  expect(input).toEqual({ ...proposal, columns: proposal.columns })
})

test('a date column without a format blocks the run until one is chosen', () => {
  const values = valuesOf(proposal)

  expect(columnsMissingDateFormat(values, fields)).toEqual([1])
})

test('only the birthday and a datetime field write a date', () => {
  expect(writesDate('attribute:birthday', fields)).toBe(true)
  expect(writesDate('customField:renews_at', fields)).toBe(true)
  expect(writesDate('customField:plan', fields)).toBe(false)
  expect(writesDate('attribute:firstName', fields)).toBe(false)
})

test('a column moved off a date drops its format, and a blank Tag is none', () => {
  const values = valuesOf(proposal)
  values.columns[2] = { target: 'customField:plan', dateFormat: 'dd/mm/yyyy' }
  values.tag = '  '

  const input = mappingInputOf(values, fields)

  expect(input.columns[2]).toEqual({ kind: 'customField', key: 'plan', dateFormat: null })
  expect(input.tag).toBeNull()
})

test('the targets are every key, every attribute and each Custom field', () => {
  const all = targets(fields)

  expect(all.keys).toContainEqual({ value: 'key:subjectId', key: 'subjectId' })
  expect(all.attributes).toContainEqual({ value: 'attribute:birthday', attribute: 'birthday' })
  expect(all.customFields.map((field) => field.value)).toEqual([
    'customField:plan',
    'customField:renews_at',
  ])
})
