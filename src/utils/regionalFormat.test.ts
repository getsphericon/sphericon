import { expect, test } from 'vitest'

import {
  formatDate,
  formatDateTime,
  formatDay,
  formatLongDate,
  formatRelative,
} from './datetime.ts'
import { formatMoney } from './money.ts'
import { formatNumber } from './number.ts'
import { formatPercent } from './percent.ts'

// The Regional format (#1010) is a locale the helpers take as an argument: the same instant and
// amount read differently in each, and nothing here reads the Instance's language. Spaces around
// a currency or percent sign are non-breaking, so they are compared as plain spaces.
const AT = '2026-12-31T13:05:09Z'
const plain = (text: string) => text.replaceAll(/\s/g, ' ')

test('a date is written the way the locale writes it', () => {
  expect(formatDate(AT, 'de-DE', 'UTC')).toBe('31.12.2026')
  expect(formatDate(AT, 'en-US', 'UTC')).toBe('12/31/2026')
  expect(formatDate(AT, 'en-GB', 'UTC')).toBe('31/12/2026')
})

test('the time of day follows the locale: 24 hours in en-GB and de-DE, 12 in en-US', () => {
  expect(plain(formatDateTime(AT, 'de-DE', 'UTC'))).toBe('31.12.2026 13:05:09')
  expect(plain(formatDateTime(AT, 'en-GB', 'UTC'))).toBe('31/12/2026 13:05:09')
  expect(plain(formatDateTime(AT, 'en-US', 'UTC'))).toBe('12/31/2026 1:05:09 PM')
})

test('the time zone moves the instant, the locale only writes it', () => {
  expect(formatDateTime(AT, 'de-DE', 'Asia/Tokyo')).toBe('31.12.2026 22:05:09')
  expect(formatDate('2026-12-31T20:00:00Z', 'de-DE', 'Asia/Tokyo')).toBe('01.01.2027')
})

test('a long date is spelled out in the locale', () => {
  expect(formatLongDate('2026-12-31T12:00:00', 'en-US')).toBe('December 31, 2026')
  expect(formatLongDate('2026-12-31T12:00:00', 'de-DE')).toBe('31. Dezember 2026')
})

test('a bare calendar day stays that day, whatever the browser’s zone', () => {
  expect(formatLongDate('1990-05-01', 'en-US')).toBe('May 1, 1990')
  expect(formatDate('1990-05-01', 'de-DE')).toBe('01.05.1990')
})

test('a calendar day is not moved by a time zone', () => {
  expect(formatDate('1990-05-01', 'de-DE', 'Pacific/Kiritimati')).toBe('01.05.1990')
  expect(formatDate('1990-05-01', 'de-DE', 'Pacific/Pago_Pago')).toBe('01.05.1990')
  expect(formatDay('2026-12-31', 'en-GB')).toBe('31/12/2026')
})

test('a number uses the locale’s grouping and decimal separator', () => {
  expect(plain(formatNumber(1_234_567.5, 'de-DE'))).toBe('1.234.567,5')
  expect(formatNumber(1_234_567.5, 'en-US')).toBe('1,234,567.5')
})

test('a relative time is worded in the locale', () => {
  const fiveMinutesAgo = Date.now() - 5 * 60 * 1000

  expect(formatRelative(fiveMinutesAgo, 'en')).toBe('5 minutes ago')
  expect(formatRelative(fiveMinutesAgo, 'de-DE')).toBe('vor 5 Minuten')
})

test('a money amount uses the locale’s separators and symbol placement', () => {
  expect(plain(formatMoney(123_450, 'EUR', 'de-DE'))).toBe('1.234,50 €')
  expect(formatMoney(123_450, 'EUR', 'en-US')).toBe('€1,234.50')
  expect(formatMoney(5000, 'JPY', 'en-US')).toBe('¥5,000')
})

test('a percentage uses the locale’s decimal separator and spacing', () => {
  expect(plain(formatPercent(0.1234, 'de-DE'))).toBe('12,34 %')
  expect(formatPercent(0.1234, 'en-US')).toBe('12.34%')
  expect(formatPercent(0.5, 'en-US', 'exceptZero')).toBe('+50%')
})
