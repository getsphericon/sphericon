import { expect, test } from 'vitest'

import { regionalFormatOptions } from './regionalFormats.ts'

test('the options are named in the UI language with a date written in each format, sorted by name', () => {
  const options = regionalFormatOptions('en', null)
  const german = options.find((option) => option.value === 'de-DE')
  const labels = options.map((option) => option.label)

  expect(german?.label).toBe('German (Germany) · 31.12.2026')
  expect(options.find((option) => option.value === 'en-GB')?.label).toBe(
    'English (United Kingdom) · 31/12/2026',
  )
  expect(labels).toEqual(labels.toSorted((a, b) => a.localeCompare(b, 'en')))
})

test('a stored format outside the list is still offered, once', () => {
  const unlisted = regionalFormatOptions('en', 'sv-FI')
  const listed = regionalFormatOptions('en', 'de-DE')

  expect(unlisted.filter((option) => option.value === 'sv-FI')).toHaveLength(1)
  expect(listed.filter((option) => option.value === 'de-DE')).toHaveLength(1)
  expect(unlisted).toHaveLength(listed.length + 1)
})
