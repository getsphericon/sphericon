import { expect, test } from 'vitest'

import type { ContactImportMapping } from '../../generated/site/types.gen.ts'
import {
  columnsMissingDateFormat,
  mappingInputOf,
  newCustomField,
  newFieldFor,
  valuesOf,
} from './importMapping.ts'

const fields = [{ key: 'plan', type: 'string' as const }]

const saved: ContactImportMapping = {
  columns: [
    { kind: 'key', key: 'email' },
    {
      kind: 'newCustomField',
      key: 'joined_on',
      label: 'Joined on',
      type: 'datetime',
      dateFormat: 'dd/mm/yyyy',
    },
  ],
  decimalSeparator: '.',
  defaultCountry: null,
  tag: null,
  onlyAddNew: false,
  asNewSignUps: false,
}

test('a new Custom field read back is the column a request saves', () => {
  const input = mappingInputOf(valuesOf(saved), fields)

  expect(input.columns).toEqual(saved.columns)
})

test('a new datetime field needs its date format, a field of another type sends none', () => {
  const values = valuesOf(saved)
  values.columns[1] = {
    target: newCustomField,
    dateFormat: null,
    newField: { key: 'joined_on', label: 'Joined on', type: 'datetime' },
  }

  expect(columnsMissingDateFormat(values, fields)).toEqual([1])

  values.columns[1] = {
    target: newCustomField,
    dateFormat: 'dd/mm/yyyy',
    newField: { key: 'visits', label: 'Visits', type: 'number' },
  }

  expect(mappingInputOf(values, fields).columns[1]).toEqual({
    kind: 'newCustomField',
    key: 'visits',
    label: 'Visits',
    type: 'number',
    dateFormat: null,
  })
})

test('a new field is proposed from its column header as text', () => {
  expect(newFieldFor(' Last purchase (EUR) ')).toEqual({
    key: 'last_purchase_eur',
    label: 'Last purchase (EUR)',
    type: 'string',
  })
})
