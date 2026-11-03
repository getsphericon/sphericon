import dayjs from 'dayjs'
import { expect, test } from 'vitest'

import { dayjsLocaleName, firstDayOfWeek, loadDayjsLocale } from './dayjsLocale.ts'

test('a Regional format reads its dates from its own dayjs locale, else its language’s, else English', () => {
  expect(dayjsLocaleName(undefined)).toBe('en')
  expect(dayjsLocaleName('en-GB')).toBe('en-gb')
  expect(dayjsLocaleName('pt-BR')).toBe('pt-br')
  expect(dayjsLocaleName('de-DE')).toBe('de')
  expect(dayjsLocaleName('es-AR')).toBe('es')
  expect(dayjsLocaleName('xx-YY')).toBe('en')
})

test('loading a locale makes its data available, and English or an unknown one needs none', async () => {
  await loadDayjsLocale('en')
  await loadDayjsLocale('unknown')
  await loadDayjsLocale('de')

  expect(dayjs.Ls['de']).toBeDefined()
  expect(dayjs.Ls['unknown']).toBeUndefined()
})

test('the calendar starts on the day the locale says', () => {
  expect(firstDayOfWeek(undefined)).toBeUndefined()
  expect(firstDayOfWeek('en-US')).toBe(0)
  expect(firstDayOfWeek('de-DE')).toBe(1)
  expect(firstDayOfWeek('ar-EG')).toBe(6)
})
